package Common

import (
	"os"
	"errors"
	"io/ioutil"
	"encoding/xml"
	. "fmt"
	"time"
)

type Configure struct {
	XMLName   xml.Name   `xml:"config"`
	System    string     `xml:"system"`
	Exchanges SExchanges `xml:"exchanges"`
}
type SExchanges struct {
	Exchange []SExchange `xml:"exchange"`
}
type SExchange struct {
	Enable        bool    `xml:"enable"`
	Name          string  `xml:"name"`
	ApiKey        string  `xml:"apiKey"`
	SecretKey     string  `xml:"secretKey"`
	RoiRate       float64 `xml:"roiRate"`
	BuyLimitMoney float64 `xml:"buyLimitMoney"`
	TimeoutBuyOrder int `xml:"timeoutBuyOrder"`
	TimeoutSellOrder int `xml:"timeoutSellOrder"`
	Coins         SCoins  `xml:"coins"`
	MaxBotNum     int     `xml:"maxBotNum"`
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
}

func Core() string {
	base := "hello core"
	Printf("%s\n", base)
	return base
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
