package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"
	tele "gopkg.in/telebot.v3"
	"gopkg.in/telebot.v3/middleware"
)

const eth = "1027"
const sol = "5426"
const pip = "34625"

type Status struct {
	Timestamp    string  `json:"timestamp"`
	ErrorCode    int     `json:"error_code"`
	ErrorMessage *string `json:"error_message"`
	Elapsed      int     `json:"elapsed"`
	CreditCount  int     `json:"credit_count"`
	Notice       *string `json:"notice"`
}

type Quote struct {
	Price                 float64 `json:"price"`
	Volume24h             float64 `json:"volume_24h"`
	VolumeChange24h       float64 `json:"volume_change_24h"`
	PercentChange1h       float64 `json:"percent_change_1h"`
	PercentChange24h      float64 `json:"percent_change_24h"`
	PercentChange7d       float64 `json:"percent_change_7d"`
	PercentChange30d      float64 `json:"percent_change_30d"`
	PercentChange60d      float64 `json:"percent_change_60d"`
	PercentChange90d      float64 `json:"percent_change_90d"`
	MarketCap             float64 `json:"market_cap"`
	MarketCapDominance    float64 `json:"market_cap_dominance"`
	FullyDilutedMarketCap float64 `json:"fully_diluted_market_cap"`
	Tvl                   *string `json:"tvl"`
	LastUpdated           string  `json:"last_updated"`
}

type CryptoData struct {
	ID                            int              `json:"id"`
	Name                          string           `json:"name"`
	Symbol                        string           `json:"symbol"`
	Slug                          string           `json:"slug"`
	NumMarketPairs                int              `json:"num_market_pairs"`
	DateAdded                     string           `json:"date_added"`
	Tags                          []string         `json:"tags"`
	MaxSupply                     *float64         `json:"max_supply"`
	CirculatingSupply             float64          `json:"circulating_supply"`
	TotalSupply                   float64          `json:"total_supply"`
	IsActive                      int              `json:"is_active"`
	InfiniteSupply                bool             `json:"infinite_supply"`
	Platform                      *string          `json:"platform"`
	CmcRank                       int              `json:"cmc_rank"`
	IsFiat                        int              `json:"is_fiat"`
	SelfReportedCirculatingSupply *float64         `json:"self_reported_circulating_supply"`
	SelfReportedMarketCap         *float64         `json:"self_reported_market_cap"`
	TvlRatio                      *string          `json:"tvl_ratio"`
	LastUpdated                   string           `json:"last_updated"`
	Quote                         map[string]Quote `json:"quote"`
}

type Response struct {
	Status Status                `json:"status"`
	Data   map[string]CryptoData `json:"data"`
}

func roundToPrecision(num float64, precision int) float64 {
	factor := math.Pow(10, float64(precision))
	return math.Round(num*factor) / factor
}

func buildPriceString(token string, price float64, change1h float64, change24h float64, change7d float64) string {
	var priceStr strings.Builder
	priceStr.WriteString(fmt.Sprintf("%s: %.2f\n", token, roundToPrecision(price, 2)))
	if change1h > 0 {
		priceStr.WriteString(fmt.Sprintf("🟩 1h:\n%.2f%%\n", roundToPrecision(change1h, 2)))
	} else {
		priceStr.WriteString(fmt.Sprintf("🟥 1h:\n%.2f%%\n", roundToPrecision(change1h, 2)))
	}
	if change24h > 0 {
		priceStr.WriteString(fmt.Sprintf("🟩 24h:\n%.2f%%\n", roundToPrecision(change24h, 2)))
	} else {
		priceStr.WriteString(fmt.Sprintf("🟥 24h:\n%.2f%%\n", roundToPrecision(change24h, 2)))
	}
	if change7d > 0 {
		priceStr.WriteString(fmt.Sprintf("🟩 7d:\n%.2f%%\n", roundToPrecision(change7d, 2)))
	} else {
		priceStr.WriteString(fmt.Sprintf("🟥 7d:\n%.2f%%\n", roundToPrecision(change7d, 2)))
	}
	priceStr.WriteString("\n")
	return priceStr.String()
}

func pollApi(bot *tele.Bot, qChan chan bool, wg *sync.WaitGroup) {
	defer wg.Done()
	chanId, err := strconv.ParseInt(os.Getenv("CHANNEL_ID"), 10, 64)
	if err != nil {
		log.Fatal(err)
		return
	}
	chat, err := bot.ChatByID(chanId)
	if err != nil {
		log.Fatal(err)
		return
	}

	for {
		select {
		case <-qChan:
			return
		default:
			client := &http.Client{}
			log.Warn("GET")
			req, err := http.NewRequest("GET", "https://pro-api.coinmarketcap.com/v1/cryptocurrency/quotes/latest", nil)
			if err != nil {
				log.Print(err)
				os.Exit(1)
			}

			q := url.Values{}
			q.Add("id", fmt.Sprintf("%s,%s,%s", eth, sol, pip))
			q.Add("convert", "USD")

			req.Header.Set("Accepts", "application/json")
			req.Header.Add("X-CMC_PRO_API_KEY", os.Getenv("CMC_TOKEN"))
			req.URL.RawQuery = q.Encode()

			resp, err := client.Do(req)
			if err != nil {
				fmt.Println("Error sending request to server")
				os.Exit(1)
			}
			var resData Response
			err = json.NewDecoder(resp.Body).Decode(&resData)
			if err != nil {
				log.Fatal("Could not decode response body")
				return
			}

			var resStr strings.Builder

			keys := make([]string, 0, len(resData.Data))
			for key := range resData.Data {
				keys = append(keys, key)
			}
			sort.Strings(keys)

			for _, key := range keys {
				value := resData.Data[key]
				resStr.WriteString(
					buildPriceString(
						value.Name,
						value.Quote["USD"].Price,
						value.Quote["USD"].PercentChange1h,
						value.Quote["USD"].PercentChange24h,
						value.Quote["USD"].PercentChange7d,
					),
				)
			}
			bot.Send(chat, resStr.String())
			time.Sleep(time.Minute * 5)
		}
	}
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	// Log as JSON instead of the default ASCII formatter.
	log.SetFormatter(&log.JSONFormatter{})

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	log.SetLevel(log.WarnLevel)

	pref := tele.Settings{
		Token:  os.Getenv("BOT_TOKEN"),
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	log.Warn("INIT")
	b, err := tele.NewBot(pref)

	if err != nil {
		log.Fatal(err)
		return
	}
	b.Use(middleware.Logger())

	var wg sync.WaitGroup
	wg.Add(1)
	qChan := make(chan (bool))
	go pollApi(b, qChan, &wg)
	wg.Wait()
}
