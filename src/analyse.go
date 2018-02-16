package main


import (
	. "fmt"
	"./api/okcoin"
	"net/http"
	api "./api"

)


func main() {
	exchange := okcoin.NewOKExSpot(http.DefaultClient,
		"0fd724ff-5cca-4eb6-acc2-1009ee58d4bc", "fc38cc52-b1d0-4ff6-abb1-4b540763a30e")

	klines, err := exchange.GetKlineRecords(api.BTC_USDT, api.KLINE_PERIOD_1MIN,2000,0)
	if err != nil {
		Printf("%s\n", err.Error())
		return
	}
	for _, record := range klines {

		Printf("%.4f\t%.4f\t%.4f\t%.4f\n", record.Open, record.High, record.Low, record.Close)
	}

	//Printf("%s\n","test")


}