package allin

import (
	api "../../api"
	. "fmt"
	. "../../common"
	"time"
)

var systemStatus *int
var exchange api.API
var config SExchange

func observe(exchange api.API)  {

	acc, err := exchange.GetAccount()
	if err != nil {
		//error
		Printf("[%s] getBalance error , err message: %s\n", TimeNow(), err.Error())
		return
	}
	balance := 0.0
	for curr, subItem := range acc.SubAccounts {

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
func Start(exc api.API, exchangeCfg SExchange, stat *int) {

	Printf("[%s] 启动%s allin bot\n", TimeNow(), exc.GetExchangeName())
	systemStatus = stat
	exchange = exc
	config = exchangeCfg
	
}
