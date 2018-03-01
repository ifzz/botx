package stratage

import (
	. "fmt"
	api "../api"
	"time"
	. "../common"
)

type CurrencyRealTimeStatus struct  {
	Enable bool //是否可以购买，是一个综合考虑的结论
	Speed int	//综合打分，成交速度、
	Price float64 //当前参考买入价格
	MarketBuyAmount float64 //当前买入总量
	MarketSellAmount float64 //当前卖出总量
	AvrBuyPrice float64 //平均买入价格
	AvrSellPrice float64 //平均卖出价格
	FinishedAmount float64 //一定时间内的成交总量
}

func observing(exchange api.API, pair api.CurrencyPair, shareObj *CurrencyRealTimeStatus) {

	//不停检查当前的行情
	var tikers = make([]api.Ticker, 0, 1000)
	for {
		time.Sleep(5 * time.Millisecond)
		ticker, err := exchange.GetTicker(pair)
		if err != nil {
			Printf("[%s] getBalance of %s error of get ticker , err message: %s\n",
				TimeNow(), pair.CurrencyA.String(), err.Error())
			continue //忽略掉
		}
		tikers = append(tikers, *ticker)


		/*
		depth, err:= exchange.GetDepth(1, pair)
		if err != nil {
			Printf("[%s] getBalance of %s error of get ticker , err message: %s\n",
				TimeNow(), pair.CurrencyA.String(), err.Error())
			continue //忽略掉
		}
		timeNow := time.Now()
		time24H := timeNow.AddDate(0,0,-1)

		klines, err:=exchange.GetKlineRecords(pair,api.KLINE_PERIOD_1MIN,1000, int(time24H.Unix()))
		if err != nil {
			Printf("[%s] getBalance of %s error of get ticker , err message: %s\n",
				TimeNow(), pair.CurrencyA.String(), err.Error())
			continue //忽略掉
		}
		*/

	}
}
func StartSingle(exchange api.API, exchangeCfg SExchange, stat *int)  {
	//检查整体行情，确定一个可买入货币

	var shareObjs [1000]CurrencyRealTimeStatus
	idx:=0
	for _, coin := range exchangeCfg.Coins.Coin {
		if coin.Enable == true {
			pair := api.CurrencyPair{api.Currency{coin.Name, coin.Name}, api.USDT}
			go observing(exchange, pair, &shareObjs[idx])
			idx++
		}
	}
	for *systemStatus == 0{

		//检查各个数字货币的状态
		for i:=0;i<idx;i++ {
			Printf("%s",shareObjs[i].Speed)
		}


		time.Sleep(5 * time.Millisecond)
	}
}

