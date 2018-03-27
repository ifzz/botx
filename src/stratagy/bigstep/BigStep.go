package bigstep

/*
	every single sell is over 10% or less 5%,
	every single buy is less 5% or over 5%
 */
import (
	api "../../api"
	. "fmt"
	. "../../common"
	"time"
)

var systemStatus *int
var exchange api.API
var config SExchange

func checking(exchange api.API, pair api.CurrencyPair) float64 {
	//一天
	klines1hour,err := exchange.GetKlineRecords(pair,"1hour","24","")
	if err != nil {
		Printf("err: %s\n", err.Error())
		return 0
	}
	time.Sleep(time.Second)
	//1小时
	klines15min,err := exchange.GetKlineRecords(pair,"15min","10","")
	if err != nil {
		Printf("err: %s\n", err.Error())
		return 0
	}
	time.Sleep(time.Second)
	//30分钟
	klines5min,err := exchange.GetKlineRecords(pair,"5min","10","")
	if err != nil {
		Printf("err: %s\n", err.Error())
		return 0
	}

	//策略为：
	/*
		最近30分钟开始涨
	*/

	var value float64 = 0

	//当前价格比12小时前不低于-5%
	value += (klines1hour[11].Open - klines1hour[0].Open) / klines1hour[0].Open
	Printf("%.4f\n", value)

	//1小时前开始涨了5%
	for i:=0;i<4;i++ {
		value += (klines15min[i+1].Open - klines15min[i].Open) / klines15min[i].Open
		Printf("%.4f\n", value)
	}
	//5分钟连续涨幅
	for i:=0;i<5;i++ {
		value += (klines5min[i+1].Open - klines5min[i].Open) / klines5min[i].Open
		Printf("%.4f\n", value)
	}
	///////30分钟连续上涨////////
	//继续上涨，斜率
	for i:=0;i<4;i++ {
		v1 := klines5min[i+2].Open - klines5min[i+1].Open
		v2 := klines5min[i+1].Open - klines5min[i].Open
		if v2 != 0 {
			value += (v1 - v2) / v2
		}
		Printf("%.4f\n", value)
	}

	return value
}

func waiting(orderid string, pair api.CurrencyPair, waitingTimeout int) bool {

	for *systemStatus == 0 && waitingTimeout > 0 {

		orderN ,err:=exchange.GetOneOrder(orderid, pair)
		if err ==nil {
			switch orderN.Status {
			case api.ORDER_FINISH:
				return true
			case api.ORDER_CANCEL:
				return false
			case api.ORDER_UNFINISH:
				return false
			}
		}
		time.Sleep(time.Second)
		waitingTimeout--

	}
	return false
}
func Start(exc api.API, exchangeCfg SExchange, stat *int) {

	Printf("[%s] 启动%s allin bot\n", TimeNow(), exc.GetExchangeName())
	systemStatus = stat
	exchange = exc
	config = exchangeCfg
	counter:=0
	for *systemStatus == 0 {

		counter = 60//等待10分钟

		//scan all coin
		//var candidatePair  api.CurrencyPair
		//amountDecimel:=""
		//priceDecimel:=""
		var canbuyValue float64 = 0
		usdtAmount := GetAvailableAmount(exchange, &api.USDT)
		if usdtAmount < config.BuyLimitMoney {
			Printf("[%s] [%s] 余额不足:%.4f \n",
				TimeNow(), exchange.GetExchangeName(), usdtAmount)
			continue
		}
		//搜索比较好的买入机会
		for _, coin:= range config.Coins.Coin {

			if coin.Enable == false {
				continue
			}

			pair := api.CurrencyPair{api.Currency{coin.Name, ""}, api.USDT}

			canbuy := checking(exchange, pair)
			Printf("%s-%.4f\n", coin.Name, canbuy)

			if canbuy > canbuyValue {
				canbuyValue = canbuy
				//candidatePair = pair
				//amountDecimel = coin.AmountDecimel
				//priceDecimel = coin.PriceDecimel
			}
			time.Sleep(time.Second)
		}
		/*
		if canbuyValue > 0 {

			ticker, err := exchange.GetTicker(candidatePair)
			if err != nil {
				continue
			}
			buyPrice := ticker.Buy + 0.1
			buyAmount := usdtAmount / buyPrice
			strbuyAmount := Sprintf(amountDecimel, buyAmount)
			strBuyPrice := Sprintf(priceDecimel, buyPrice)

			time.Sleep(131 * time.Millisecond)
			order, err := exchange.LimitBuy(strbuyAmount, strBuyPrice, candidatePair)
			if nil == err {
				Printf("[%s] [%s] 挂买入单成功，订单号 : %d, 价格:%s /%s \n",
					TimeNow(), exchange.GetExchangeName(), order.OrderID, strBuyPrice, strbuyAmount)

				if true == waiting(order.OrderID2, candidatePair, 600) {
					Printf("[%s] [%s] 买入成功，订单号 : %d, 价格:%s /%s \n",
						TimeNow(), exchange.GetExchangeName(), order.OrderID, strBuyPrice, strbuyAmount)
					//买入成功，设置卖出5%
					sellPrice := buyPrice * 1.05
					strSellPrice := Sprintf(priceDecimel, sellPrice)
					orderSell, err := exchange.LimitSell(strbuyAmount, strSellPrice, candidatePair)
					if nil == err {
						//等待1天
						if true == waiting(orderSell.OrderID2, candidatePair, 3600*12) {
							Printf("[%s] [%s] 卖出成功，订单号 : %d, 价格:%s /%s \n",
								TimeNow(), exchange.GetExchangeName(), orderSell.OrderID, strSellPrice, strbuyAmount)
						}else {
							Printf("[%s] [%s] 等待卖出状态失败(1天未卖出) :%s 价格:%s /%s \n",
								TimeNow(), exchange.GetExchangeName(), err.Error(), strSellPrice, strbuyAmount)
						}
					}else {
						Printf("[%s] [%s] 挂卖出单失败(1800s) :%s 价格:%s /%s \n",
							TimeNow(), exchange.GetExchangeName(), err.Error(), strSellPrice, strbuyAmount)
					}
				}else {
					Printf("[%s] [%s] 等待买入状态失败 :%s 价格:%s /%s \n",
						TimeNow(), exchange.GetExchangeName(), err.Error(), strBuyPrice, strbuyAmount)
					continue//如果买入失败，可能是设置买入价格不合适，再尝试买入一次
				}

			} else {
				Printf("[%s] [%s] 挂买入单失败 :%s 价格:%s /%s \n",
					TimeNow(), exchange.GetExchangeName(), err.Error(), strBuyPrice, strbuyAmount)
			}
		}
		*/

		//每分钟进行一次买入/卖出操作
		for *systemStatus == 0 && counter > 0{
			counter--
			time.Sleep(time.Second)
		}
	}

}
