package stratage

import (
	api "../api"
	. "fmt"
	. "../common"
	"time"
)

var exchange api.API
var config SExchange
var orderList map[int] OrderInfo //存储所有的order
var orderMap map[int] int //买-卖订单倒排列表

func whileBuying()  {

	for *systemStatus == 0{
		//checking account
		for _, coin:= range config.Coins.Coin {
			if coin.Enable == false {
				continue
			}

			for buyOrderID, sellOrderID := range orderMap {

				buyOrder := orderList[buyOrderID]

				if sellOrderID != 0 {//已经挂卖出单，直接忽略
					continue
				}

				if buyOrder.Currency.Symbol != coin.Name {//不是当前判断货币
					continue
				}

				if buyOrder.Status == ORDERCANCEL {//买入单已经取消
					continue
				}
				coinAvAmount := GetAvailableAmount(exchange, &api.Currency{coin.Name,""})

				if coinAvAmount >= buyOrder.Amount {
					//可以卖出
					pair := api.CurrencyPair{api.Currency{coin.Name, ""}, api.USDT}

					ticker,err := exchange.GetTicker(pair)
					price := buyOrder.Price * (1 + config.RoiRate * 2)
					if err == nil {
						if price < ticker.Last {
							price = ticker.Last
						}
					}

					amount := buyOrder.Amount
					strPrice := Sprintf(coin.PriceDecimel,price)
					strAmout := Sprintf(coin.AmountDecimel,amount)
					sellOrder,err := exchange.LimitSell(strAmout, strPrice, pair)
					if err == nil {

						currency := api.Currency{coin.Name, ""}
						orderInfo := OrderInfo{sellOrder.OrderID, api.ToFloat64(price),
						api.ToFloat64(amount), ORDERPAIRFINISH, currency, api.SELL}
						orderList[sellOrder.OrderID] = orderInfo

						orderMap[buyOrder.ID] = sellOrder.OrderID//更新ID
						Printf("[%s] [%s] sell order, orderpair(%d:%d), price:%s, amount:%s\n",
							TimeNow(), coin.Name,buyOrder.ID, sellOrder.OrderID, strPrice, strAmout)

					}else {
						Printf("[%s] sell one put order failed, err: %s\n", TimeNow(), err.Error())
					}
					time.Sleep(time.Second)
				}
			}

		}
		time.Sleep(2 * time.Second)
	}
}
func updateOrderStatus()  {

	for *systemStatus == 0{

		for id, order := range orderList {
			if order.Status == ORDERFINISHED || order.Status == ORDERCANCEL {
				continue
			}
			strID := Sprintf("%d",id)
			exOrder, err := exchange.GetOneOrder(strID, api.CurrencyPair{order.Currency,api.USDT})
			if err == nil {
				switch exOrder.Status {
				case api.ORDER_FINISH:
					Printf("[%s] update order status to finished, orderid:%d, price:%.4f, amount:%.4f, side:%d\n",
						TimeNow(), order.ID, order.Price, order.Amount, order.Side)
					order.Status = ORDERFINISHED
					orderList[id] = order
				case api.ORDER_CANCEL:
					Printf("[%s] update order status to cancel, orderid:%d, price:%.4f, amount:%.4f, side:%d\n",
						TimeNow(), order.ID, order.Price, order.Amount, order.Side)
					order.Status = ORDERCANCEL
					orderList[id] = order
				case api.ORDER_UNFINISH:

				}
				time.Sleep(time.Second)//间隔1秒
			}
		}
		counter :=  60
		for *systemStatus == 0 && counter > 0 {
			counter--
			time.Sleep(time.Second) //每分钟更新一次
		}
	}
}
func showROIBalance()  {

	var balanceBeginUSDT float64 = 0
	balanceBeginUSDT = api.ToFloat64(GetBalance(exchange, &api.USDT))//USDT余额
	var balanceBeginCoins float64 = 0
	var coinsPriceMap map[string] PriceInfo = make(map[string] PriceInfo)
	for _, coin := range config.Coins.Coin {
		if coin.Enable == false {
			continue
		}
		balanceBeginCoins += api.ToFloat64(GetBalance(exchange, &api.Currency{coin.Name,""}))
		var priceBegin PriceInfo
		ticker,err:= exchange.GetTicker(api.CurrencyPair{api.Currency{coin.Name, ""},api.USDT})
		if err == nil {
			priceBegin.PriceBegin = ticker.Last
		}else {
			priceBegin.PriceBegin = 0.0001
		}
		coinsPriceMap[coin.Name] = priceBegin
	}

	for *systemStatus == 0{

		totalOrder := 0
		totalPair := 0
		totalFinishedPair := 0
		totalUnfinishedSellOrder := 0
		totalUnfinishedBuyOrder := 0
		finishedOrder := 0
		cancelOrder := 0

		finishedBuyOrder := 0
		finishedSellOrder := 0
		totalOrder = len(orderList)
		totalPair = len(orderMap)
		for idBuy, idSell := range orderMap {
			buyOrder := orderList[idBuy]
			sellOrder := orderList[idSell]
			if buyOrder.Status == ORDERFINISHED && sellOrder.Status == ORDERFINISHED {
				totalFinishedPair++
				continue
			}
			if buyOrder.Status == ORDERWAITING {
				totalUnfinishedBuyOrder++
			}
			if sellOrder.Status == ORDERWAITING {
				totalUnfinishedSellOrder++
			}
			if buyOrder.Status == ORDERCANCEL || sellOrder.Status == ORDERCANCEL {
				cancelOrder++
			}

			if buyOrder.Status == ORDERFINISHED || sellOrder.Status == ORDERFINISHED {
				finishedOrder++
			}
			if buyOrder.Status == ORDERFINISHED {
				finishedBuyOrder++
			}
			if sellOrder.Status == ORDERFINISHED {
				finishedOrder ++
			}
		}

		//获取盈利情况//计算收益
		balanceCurrentUSDT := api.ToFloat64(GetBalance(exchange, &api.USDT))

		var balanceCurrentCoins float64 = 0
		coinNames := "USDT"
		coinPriceInfo := ""
		var priceCurr float64 = 0.0001
		for _, coin := range config.Coins.Coin {
			if coin.Enable == false {
				continue
			}
			coinNames = coinNames + "-" + coin.Name
			balanceCurrentCoins += api.ToFloat64(GetBalance(exchange, &api.Currency{coin.Name, ""}))
			pair := api.CurrencyPair{api.Currency{coin.Name, ""}, api.USDT}
			tickerCurr, err := exchange.GetTicker(pair)
			if err == nil {
				priceCurr = tickerCurr.Last
			}

			priceInfo := coinsPriceMap[pair.CurrencyA.Symbol]
			priceInfo.PriceCurrent = priceCurr
			coinsPriceMap[pair.CurrencyA.Symbol] = priceInfo
			coinPriceInfo += Sprintf("%s币价从%.4f到%.4f,变化率:%.4f%%),",
				pair.CurrencyA.Symbol, priceInfo.PriceBegin, priceInfo.PriceCurrent,
				(priceInfo.PriceCurrent-priceInfo.PriceBegin)*100/priceInfo.PriceBegin)

		}
		rate := (balanceCurrentUSDT + balanceCurrentCoins - balanceBeginCoins - balanceBeginUSDT) / (balanceBeginCoins + balanceBeginUSDT + 0.0000001)
		rate = rate * 100

		Printf("[%s] [%s]有效货币(%s),开始余额:%.4f,当前余额:%.4f,累积收益:%.4f%%,总交易对:%d,完成交易对:%d,待成交订单:%d,完成订单:%d,取消订单:%d,总订单:%d,%s currentUSDT:%.4f, 完成买入单:%d, 完成卖出单:%d\n",
			TimeNow(), exchange.GetExchangeName(), coinNames,
			balanceBeginUSDT+balanceBeginCoins,
			balanceCurrentUSDT+balanceCurrentCoins, rate,
			totalPair, totalFinishedPair,
			totalUnfinishedBuyOrder+totalUnfinishedSellOrder,
			finishedOrder, cancelOrder,
			totalOrder,
			coinPriceInfo, balanceCurrentUSDT,
				finishedBuyOrder, finishedSellOrder)

		counter := 5 * 60
		for *systemStatus == 0 && counter > 0 {
			counter--
			time.Sleep(time.Second)
		}
	}
}
//启动一个交易平台
var systemStatus *int

func StartDouble(exc api.API, exchangeCfg SExchange, stat *int) {

	Printf("[%s] 启动%s bot\n", TimeNow(), exc.GetExchangeName())
	systemStatus = stat
	exchange = exc
	config = exchangeCfg

	orderList = make(map[int] OrderInfo)
	orderMap = make(map[int] int)

	go showROIBalance()
	go whileBuying()
	go updateOrderStatus()

	tradeFrequency := time.Duration(config.TradeFrequency)
	for *systemStatus == 0{

		for _, coin:= range config.Coins.Coin {
			if coin.Enable == false {
				continue
			}
			usdtAmount := GetAvailableAmount(exchange, &api.USDT)
			if usdtAmount < config.BuyLimitMoney {
				continue
			}
			pair := api.CurrencyPair{api.Currency{coin.Name, ""}, api.USDT}
			ticker,err := exchange.GetTicker(pair)
			if err !=nil {
				Printf("err:\n", err.Error())
				continue
			}
			price:=ticker.Last

			/*
			depth, err:= exchange.GetDepth(50, pair)
			if err == nil {

				avAskPrice := 0.0
				avAskAmount := 0.0
				avBidPrice := 0.0
				avBidAmount := 0.0
				cnt := 0
				//卖方深度
				for _, ask := range depth.AskList {
					avAskPrice += ask.Price * ask.Amount
					avAskAmount += ask.Amount
					cnt++
				}
				avAskPrice = avAskPrice / avAskAmount

				//买方深度
				for _, bid := range depth.BidList {
					avBidPrice += bid.Price * bid.Amount
					avBidAmount += bid.Amount
				}
				avBidPrice = avBidPrice / avBidAmount
			}
			*/

			//askPrice := price * (1 + config.RoiRate)
			bidPrice := price * (1 - config.RoiRate)
			//askAmount := config.BuyLimitMoney / bidPrice
			bidAmount := config.BuyLimitMoney / bidPrice //买卖的量保持一致
			if coin.LimitAmount > 0 {
				//按量购买
				//askAmount = coin.LimitAmount
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