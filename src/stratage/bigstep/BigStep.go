package bigstep

import (
	api "../../api"
	. "fmt"
	. "../../common"
	"time"
)

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

	orderList = make(map[int]OrderInfo)
	orderMap = make(map[int]int)

	tradeFrequency := time.Duration(config.TradeFrequency)
	for *systemStatus == 0 {

		for _, coin := range config.Coins.Coin {
			if coin.Enable == false {
				continue
			}
			usdtAmount := GetAvailableAmount(exchange, &api.USDT)
			if usdtAmount < config.BuyLimitMoney {
				continue
			}
			pair := api.CurrencyPair{api.Currency{coin.Name, ""}, api.USDT}

			bidPrice := getBuyPrice(pair)
			if bidPrice == 0 {
				continue
			}
			availableMoney := usdtAmount / float64(exchangeCfg.AverageNum)

			bidAmount := availableMoney / bidPrice //买卖的量保持一致
			if coin.LimitAmount > 0 {
				//按量购买
				bidAmount = coin.LimitAmount
			}

			//strAskPrice := Sprintf(coin.PriceDecimel,askPrice)
			strBidPrice := Sprintf(coin.PriceDecimel, bidPrice)

			//strAskAmount := Sprintf(coin.AmountDecimel, askAmount)
			strBidAmount := Sprintf(coin.AmountDecimel, bidAmount)

			buyOrder,err := exchange.LimitBuy(strBidAmount, strBidPrice, pair)
			if err != nil {
				Printf("buy one put order failed, err: %s\n", err.Error())
			}else {
				currency := api.Currency{coin.Name, ""}
				orderInfo := OrderInfo{buyOrder.OrderID, api.ToFloat64(strBidPrice),
					api.ToFloat64(strBidAmount), ORDERWAITING, currency, api.BUY}
				orderList[buyOrder.OrderID] = orderInfo
				orderMap[buyOrder.OrderID] = 0 //先设置为0

				Printf("[%s] [%s] buy order, orderpair(%d:-), price:%s, amount:%s\n",
					TimeNow(), coin.Name, buyOrder.OrderID, strBidPrice, strBidAmount)
			}

		}

		//每分钟进行一次买入/卖出操作
		counter := tradeFrequency * 60
		for *systemStatus == 0 && counter > 0{
			counter--
			time.Sleep(time.Second)
		}
	}
}
