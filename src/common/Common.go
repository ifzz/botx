package Common

import (
	"os"
	"errors"
	"io/ioutil"
	"encoding/xml"
	. "fmt"
	"time"
	"../api"
)

type Configure struct {
	XMLName   xml.Name   `xml:"config"`
	System    string     `xml:"system"`
	Exchanges SExchanges `xml:"exchanges"`
}
type SExchanges struct {
	Exchange []SExchange `xml:"exchange"`
}
const (
	MODE_MONEY = 1	//"money"
	MODE_COIN  =2	// "coin"
)
type SExchange struct {
	Enable        bool    `xml:"enable"`
	Name          string  `xml:"name"`
	ApiKey        string  `xml:"apiKey"`
	SecretKey     string  `xml:"secretKey"`
	RoiRate       float64 `xml:"roiRate"`
	BuyLimitMoney float64 `xml:"buyLimitMoney"`
	TimeoutBuyOrder int `xml:"timeoutBuyOrder"` //second
	TimeoutSellOrder int `xml:"timeoutSellOrder"` //second
	Coins         SCoins  `xml:"coins"`
	MaxBotNum     int     `xml:"maxBotNum"`
	Mode     int 	`xml:"mode"`
	BotTimeSpan int `xml:"botTimeSpan"` //second
	WaitingQueue int `xml:"waitingQueue"`
	FreeUseQueue int `xml:"freeUseQueue"`
	AverageNum int `xml:"averageNum"`
	TradeFrequency int `xml:"tradeFrequency"`
}
type SCoins struct {
	Coin []SCoin `xml:"coin"`
}

type SCoin struct {
	Enable        bool   `xml:"enable"`
	Name          string `xml:"name"`
	Pair          string `xml:"pair"`
	PriceDecimel  string `xml:"priceDecimel"`
	AmountDecimel string `xml:"amountDecimel"`
	LimitAmount float64 `xml:"limitAmount"`
}

func Core() string {
	base := "hello core"
	Printf("%s\n", base)
	return base
}

const ORDERFINISHED  = 0
const ORDERWAITING = 1
const ORDERCANCEL = 2
const ORDERPAIRFINISH = 3 //表示买入-卖出对完成

type OrderInfo struct {
	ID int
	Price float64
	Amount float64
	Status int //0-finished,1-waiting,2-cancel
	Currency api.Currency
}

type PriceInfo struct {
	PriceBegin float64
	PriceCurrent float64
}


func TimeNow() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func LoadConfigure(filePath string) (Configure, error) {

	var cfg Configure
	file, err := os.Open(filePath) // For read access.
	if err != nil {
		Printf("error: %v", err)
		return cfg, errors.New("open config file failed")
	}
	defer file.Close()
	data, err := ioutil.ReadAll(file)
	if err != nil {
		Printf("error: %v", err)
		return cfg, errors.New("read file failed")
	}

	err = xml.Unmarshal(data, &cfg)
	if err != nil {
		Printf("error: %v", err)
		return cfg, errors.New("xml parse failed")
	}
	return cfg, nil
}

//获取可用的数字货币数量
func GetAvailableAmount(exchange api.API, currency *api.Currency) float64 {
	time.Sleep(131 * time.Millisecond)
	acc, err := exchange.GetAccount()
	if err != nil {
		//error
		Printf("[%s] getBalance error , err message: %s\n", TimeNow(), err.Error())
		return 0
	}
	amount := 0.0
	for curr, subItem := range acc.SubAccounts {

		if curr.Symbol == currency.Symbol {
			amount = subItem.Amount
			break
		}

	}
	return amount
}

//获取账户总额
func GetBalance(exchange api.API, currency *api.Currency) string {
	time.Sleep(131 * time.Millisecond)
	acc, err := exchange.GetAccount()
	if err != nil {
		//error
		Printf("[%s] getBalance error , err message: %s\n", TimeNow(), err.Error())
		return "0.0000"
	}
	balance := 0.0
	for curr, subItem := range acc.SubAccounts {
		if currency != nil {

			if curr.Symbol == currency.Symbol {
				amount := subItem.Amount + subItem.ForzenAmount

				if amount > 0 {
					if curr == api.USDT { //如果是USDT，直接退出循环
						balance += amount
						break
					}
					time.Sleep(131 * time.Millisecond)
					ticker, err := exchange.GetTicker(api.CurrencyPair{*currency, api.USDT})
					if err != nil {
						Printf("[%s] getBalance of %s error of get ticker , err message: %s\n",
							TimeNow(), curr.String(), err.Error())
						return "0.0000"
					}
					balance += amount * ticker.Last
				}

				break
			}
		} else { //get full
			amount := subItem.Amount + subItem.ForzenAmount

			if amount > 0 {
				if curr != api.USDT {
					ticker, err := exchange.GetTicker(api.CurrencyPair{curr, api.USDT})
					if err != nil {
						Printf("[%s] getBalance of %s error of get ticker , err message: %s\n",
							TimeNow(), curr.String(), err.Error())
						continue //忽略掉
					}
					//Printf("%s,%.4f\n",curr.String(), amount)
					balance += amount * ticker.Last
				} else {
					balance += amount
				}
			}
			time.Sleep(137 * time.Millisecond)
		}
	}
	return Sprintf("%.4f", balance)

}
