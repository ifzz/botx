package main

import (
	api "./api"
	"./api/okcoin"
	"./api/zb"
	"./stratage"
	. "./common"
	"errors"
	. "fmt"
	"math/rand"
	"net/http"
	"time"

	"flag"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"math"
)

var systemExit bool = false

type OrderInfo struct {
	Price float64
	Amount float64
	Status int //0-finished,1-waiting,2-cancel
}
type Bot struct {
	ID           int              // BotID
	LimitMoney       float64       //当前账户使用金额
	LimitAmount 	float64 	//limit buy in amount, if it's 0, LimitMoney will be used
	Price        float64          //当前价格
	Type         int              //类型: 0，1分别代表买入，卖出
	Timestamp    time.Time        //当前时间
	Status       int              //当前状态:0，1，2分别代表
	RoiRate      float64          //收益率
	Counter      int              //完成个数
	CurrencyPair api.CurrencyPair //
	Exchange     api.API          //
	/*damping = 1/(avr_time + count + 1)*/
	Damping float32 //阻尼系数，表示该bot运行健康度，成交对间隔越短、次数越多，系数越低

	//不同交易所，不同交易货币，精度不一样
	PriceDecimel  string //价格精度
	AmountDecimel string //数量精度

	Name      string
	StartTime time.Time //启动时间

	OrderList map[int] OrderInfo //key is 订单id
	OrderPair map[int] int //key:买入订单id, 卖出订单id
}

//买入
func BuyIn(money float64, amount float64, latestOrder *api.Order, bot *Bot, roiRateCfg float64) (*api.Order, error) {
	retErr := errors.New(TimeNow() + "挂买单失败")

	buyPrice := calcBuyPrice(bot.Exchange, bot.CurrencyPair,roiRateCfg)
	if buyPrice == 0 {
		Printf("[%s] [%s %s-USDT-bot %d] 获取买入价格失败\n",
			TimeNow(), bot.Exchange.GetExchangeName(), bot.Name, bot.ID)
		return nil, retErr
	}

	buyAmount:= amount
	if buyAmount == 0 {//如果不是按照量买入，则根据买入现金计算买入量，否则直接按照最小买入量
		buyAmount = money / buyPrice
	}

	strbuyAmount := Sprintf(bot.AmountDecimel, buyAmount)
	strBuyPrice := Sprintf(bot.PriceDecimel, buyPrice)
	//xx,_:=strconv.ParseFloat(strBuyPrice, 32)
	//Printf("%.4f\n", xx)

	time.Sleep(131* time.Millisecond)
	order, err := bot.Exchange.LimitBuy(strbuyAmount, strBuyPrice, bot.CurrencyPair)
	if nil == err {
		Printf("[%s] [%s %s-USDT-bot %d] 挂买入单 ok : %d, 价格:%s /%s \n",
			TimeNow(), bot.Exchange.GetExchangeName(), bot.Name,bot.ID, order.OrderID, strBuyPrice, strbuyAmount)
		return order, nil

	} else {
		if err.Error() != "2009" { //2009 余额不足
			Printf("[%s] [%s %s-USDT-bot %d] 挂买入单 err:%s, 价格:%s /%s \n",
				TimeNow(), bot.Exchange.GetExchangeName(), bot.Name, bot.ID, err.Error(), strBuyPrice, strbuyAmount)
		}

		return order, err
	}
	return order, retErr
}

//计算可以买入的价格
func calcBuyPrice(exchange api.API, pair api.CurrencyPair, roiRate float64) float64  {
	var price float64 = 0
	time.Sleep(125* time.Millisecond)
	ticker, err := exchange.GetTicker(pair)
	if err != nil {
		return price
	}

	price = ticker.Buy
	time.Sleep(171* time.Millisecond)
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

		if roiRate * price > avAskPrice {
			//根据现在的roiRate，卖出价格超出了平均卖方价格，考虑暂时不做买入操作
			Printf("[%s] [%s USDT] calcBuy Price err 1 价格:%.4f / %.4f \n",
				TimeNow(),exchange.GetExchangeName(),price,avAskPrice)
			price = 0
			return price
		}

		base2BidPrice := depth.BidList[1].Price
		base8AskPrice := depth.AskList[7].Price

		if roiRate * base2BidPrice > base8AskPrice {
			//根据现在的roiRate，卖出价格超出了平均卖方价格，考虑暂时不做买入操作
			Printf("[%s] [%s USDT] calcBuy Price err 2 价格:%.4f / %.4f \n",
				TimeNow(),exchange.GetExchangeName(),base2BidPrice,base8AskPrice)
			price = 0
			return price
		}
		price = math.Min(base2BidPrice, price)//使用买方第2哥价格
	}
	price += 0.01
	return price

}

//计算可以卖出的价格
func calcSellPrice(depth api.Depth, orignPrice float64, minPrice float64) float64  {

	var price float64 = orignPrice
	avAskPrice := 0.0
	avAskAmount := 0.0
	avBidPrice := 0.0
	avBidAmount := 0.0
	cnt := 0
	//卖方深度
	for _,ask:= range depth.AskList {
		avAskPrice += ask.Price*ask.Amount
		avAskAmount += ask.Amount
		cnt++
	}
	avAskPrice = avAskPrice / avAskAmount

	//买方深度
	for _, bid := range depth.BidList {
		avBidPrice += bid.Price*bid.Amount
		avBidAmount += bid.Amount
	}
	avBidPrice = avBidPrice / avBidAmount

	base20Price := depth.AskList[19].Price
	if orignPrice > avAskPrice || orignPrice > base20Price {
		//如果当前设定的价格过高: 高于平均卖单价格，或者高于第20层的价格，调低价格
		tmp := math.Min(avAskPrice, base20Price)
		price = math.Max(minPrice, tmp)
	}
	return price
}

//卖出一个订单
func SellOut(latestOrder *api.Order, bot *Bot, speed int64, roiCfgRate float64, mode int) (*api.Order, error) {

	//以赚取coin为目标
	retErr := errors.New("挂卖单失败")

	roiRate := roiCfgRate
	//如果5分钟内成交，可以增大收益率
	if speed < 60 { //1分钟
		roiRate = roiCfgRate * 5
	} else if speed < 120 { //如果2分钟内成交，可以增大收益率
		roiRate = roiCfgRate * 3
	} else if speed < 300 { //5分钟
		roiRate = roiCfgRate * 2
	} else if speed < 600 { //10分钟
		roiRate = roiCfgRate * 1.5
	} else {
		roiRate = roiCfgRate
	}
	sellPrice := latestOrder.Price * (1 + roiRate)
	time.Sleep(101* time.Millisecond)
	ticker, err := bot.Exchange.GetTicker(bot.CurrencyPair)
	if err == nil {
		if ticker.Sell > sellPrice { //如果收益计算后比当前市场卖价格低，直接挂市场卖价
			sellPrice = ticker.Sell
		}
	}
	sellPrice -= 0.01
	time.Sleep(107* time.Millisecond)
	depth, err:= bot.Exchange.GetDepth(50, bot.CurrencyPair)
	if err == nil {
		sellPrice = calcSellPrice(*depth, sellPrice, latestOrder.Price * roiCfgRate)
	}
	//2018/2/15 根据mode判断卖出价格，TODO，可能有float精度损失
	strSellAmount := "0.0"
	availableAmount := getAvailableAmount(bot.Exchange, &bot.CurrencyPair.CurrencyA)
	if mode == MODE_COIN {

		sellAmount := latestOrder.Amount / (1 + roiRate)
		Printf("[%s] [%s %s-USDT-bot %d] mode=coin, last amount:%.4f, sell amount:%.4f\n",
			TimeNow(), bot.Exchange.GetExchangeName(), bot.Name, bot.ID, latestOrder.Amount, sellAmount)
		if availableAmount >= sellAmount {
			strSellAmount = Sprintf(bot.AmountDecimel,sellAmount)
		}else {
			Printf("[%s] [%s %s-USDT-bot %d] mode=coin 可用coin不足,且无法满足收益，当前可用:%.4f, 需要:%.4f \n",
				TimeNow(), bot.Exchange.GetExchangeName(), bot.Name, bot.ID, availableAmount, sellAmount)
			return nil, errors.New("可用coin不足，且无法满足收益")
		}

	}else {
		Printf("[%s] [%s %s-USDT-bot %d] mode=money\n",
			TimeNow(), bot.Exchange.GetExchangeName(), bot.Name, bot.ID)
		if availableAmount >= latestOrder.Amount {//可用量足够

			strSellAmount = Sprintf(bot.AmountDecimel, latestOrder.Amount)//默认使用卖出量

		} else if availableAmount < latestOrder.Amount && availableAmount >= latestOrder.Amount / (1 + roiRate) {

			//能满足收益，按可用量挂单
			strSellAmount = Sprintf(bot.AmountDecimel, availableAmount)

		}else {//如果 coin不足，且收益无法保障
			Printf("[%s] [%s %s-USDT-bot %d] mode=money 可用coin不足,且无法满足收益，当前可用:%.4f, 需要:%.4f \n",
				TimeNow(), bot.Exchange.GetExchangeName(), bot.Name, bot.ID, availableAmount, latestOrder.Amount)
			return nil, errors.New("可用coin不足，且无法满足收益")
		}
	}

	strSellPrice := Sprintf(bot.PriceDecimel, sellPrice)
	time.Sleep(109* time.Millisecond)
	order, err := bot.Exchange.LimitSell(strSellAmount, strSellPrice, bot.CurrencyPair)
	if nil == err {
		Printf("[%s] [%s %s-USDT-bot %d] 挂卖出单 ok : %d，价格:%s / %s \n",
			TimeNow(), bot.Exchange.GetExchangeName(), bot.Name, bot.ID, order.OrderID, strSellPrice, strSellAmount)
		//put sell order
		return order, nil
	} else {
		Printf("[%s] [%s %s-USDT-bot %d] 挂卖出单err:%s, 价格:%s /%s \n",
			TimeNow(), bot.Exchange.GetExchangeName(), bot.Name,bot.ID,  err.Error(), strSellPrice, strSellAmount)
	}
	return order, retErr


}

///取消订单
func tryCancelOrder(latestOrder *api.Order, bot *Bot) (bool, error) {

	shouldCancel := false
	retErr := errors.New("挂取消单失败")
	orderID := latestOrder.OrderID2
	time.Sleep(117* time.Millisecond)
	ticker, err := bot.Exchange.GetTicker(bot.CurrencyPair)
	if err != nil {
		Printf("[%s] [%s %s-USDT-bot %d] 获取Ticker出错，message: %s\n",
			TimeNow(), bot.Exchange.GetExchangeName(), bot.Name,bot.ID, err.Error())
		return shouldCancel, retErr
	}
	time.Sleep(108* time.Millisecond)
	depth, err:= bot.Exchange.GetDepth(50, bot.CurrencyPair)
	idxDepth := 0

	if err == nil {
		//买方深度
		for idx, bid := range depth.BidList {
			if bot.Price > bid.Price {
				idxDepth = idx
				break
			}
		}
	}

	if (ticker.Buy / bot.Price) > 0.02 || idxDepth > 5 {
		//超过2%，或者买入深度已经埋没到超过5层，可以取消订单
		Printf("[%s] [%s %s-USDT-bot %d] 取消订单，买入价格:%.4f, 现价: %.4f\n",
			TimeNow(), bot.Exchange.GetExchangeName(), bot.Name, bot.ID, bot.Price, ticker.Buy)
		shouldCancel = true
		time.Sleep(115* time.Millisecond)
		_, err := bot.Exchange.CancelOrder(orderID, bot.CurrencyPair)
		if nil == err {
			//成功
			return shouldCancel, nil
		} else {
			Printf("[%s] [%s %s-USDT-bot %d] 取消订单失败, err:%s\n",
				TimeNow(), bot.Exchange.GetExchangeName(), bot.Name, bot.ID, err.Error())
		}
	}

	return shouldCancel, retErr

}

///程序启动一个bot
func Start(bot *Bot, exchangeCfg SExchange) {

	Printf("[%s] [%s %s-USDT-bot %d] start a new bot \n",
		TimeNow(), bot.Exchange.GetExchangeName(), bot.Name, bot.ID)

	var orderID string = ""

	var speed int64 = 1000000 ////speed 挂卖出单，到卖出单交易成功的时间间隔

	//计算当前bot的收益情况
	var counterBuyin int64 = 0
	var counterSellout int64 = 0
	var counterBuyinMoney float64 = 0
	var counterSelloutMoney float64 = 0
	//orderMap := make(map[string]string) //记录所有成交对
	var updateTimer = 0
	for systemExit == false {

		time.Sleep(1539 * time.Millisecond)

		acct, err := bot.Exchange.GetAccount()
		if err != nil {
			msg:=err.Error()
			if len(msg) > 100 {
				msg = msg[0:100]
			}
			Printf("[%s] [%s %s-USDT-bot %d] 获取账户出错，继续， 信息:%s\n",
				TimeNow(), bot.Exchange.GetExchangeName(), bot.Name, bot.ID, msg)
			continue
		}

		currentUSDTAmount := acct.SubAccounts[api.USDT].Amount

		//检查订单状态
		if orderID != "" {
			time.Sleep(131 * time.Millisecond)
			latestOrder, err := bot.Exchange.GetOneOrder(orderID, bot.CurrencyPair)
			if err != nil || latestOrder == nil {
				Printf("[%s] [%s %s-USDT-bot %d] 读取订单(%s)状态失败:%s\n",
					TimeNow(), bot.Exchange.GetExchangeName(), bot.Name,bot.ID, orderID, err.Error())
				continue
			}
			/******DUEBUG**
			Printf("orderID:%d (price:%.4f, amount:%.4f, fee:%.4f, status:%d)\n",
				latestOrder.OrderID, latestOrder.Price,latestOrder.Amount,
					latestOrder.Fee, latestOrder.Status)
			*/

			if latestOrder.Status == api.ORDER_FINISH && latestOrder.Side == api.BUY {
				//订单完成，如果是买入订单，则可以挂卖单
				//set status to finish
				item, exist:= bot.OrderList[latestOrder.OrderID]
				if exist == true {
					item.Status = 0
					bot.OrderList[latestOrder.OrderID] = item //modiy
				}

				Printf("[%s] [%s %s-USDT-bot %d] 交易（买入）成功，订单号:%d\n",
					TimeNow(), bot.Exchange.GetExchangeName(), bot.Name, bot.ID, latestOrder.OrderID)

				//Println(TimeNow() + "订单完成，如果是买入订单，则可以挂卖单")
				currentOrder, cerr := SellOut(latestOrder, bot, speed, exchangeCfg.RoiRate, exchangeCfg.Mode)
				if cerr == nil {
					Printf("[%s] [%s %s-USDT-bot %d] couple (buy-sell),orderid:(%d-%d), price:(%.4f,%.4f), amount:(%.4f,%.4f), rate:%.4f %%\n",
						TimeNow(), bot.Exchange.GetExchangeName(), bot.Name, bot.ID,
							latestOrder.OrderID, currentOrder.OrderID,
						latestOrder.Price, currentOrder.Price,
							latestOrder.Amount, currentOrder.Amount,
						100 * (currentOrder.Price - latestOrder.Price) / latestOrder.Price)

					orderID = currentOrder.OrderID2 //Sprintf("%d", currentOrder.OrderID) //保存最新ID

					//完成时间
					bot.Timestamp = time.Now()

					counterSellout++
					counterSelloutMoney += currentOrder.Price * currentOrder.Amount

					//orderMap[latestOrder.OrderID2] = currentOrder.OrderID2
					bot.OrderPair[latestOrder.OrderID] = currentOrder.OrderID

					orderInfo := OrderInfo{currentOrder.Price, currentOrder.Amount,1}
					bot.OrderList[currentOrder.OrderID] = orderInfo //insert one new

					//TODO TEST 交易完成一次，则退出
					//break
				}

			} else if latestOrder.Status == api.ORDER_FINISH && latestOrder.Side == api.SELL {

				//订单完成，如果是卖出订单，可以挂买单
				//set status to finish
				item, exist:= bot.OrderList[latestOrder.OrderID]
				if exist == true {
					item.Status = 0
					bot.OrderList[latestOrder.OrderID] = item //modiy
				}

				Printf("[%s] [%s %s-USDT-bot %d] 交易（卖出）成功，订单号:%d\n",
					TimeNow(), bot.Exchange.GetExchangeName(), bot.Name, bot.ID, latestOrder.OrderID)

				//speed 挂卖出单，到卖出单交易成功的时间间隔
				speed = time.Now().Unix() - bot.Timestamp.Unix()

				//TODO,仓位管理，如果小于80%仓位，不要买入，不能满仓
				//TODO,对于完成很快的bot，适当调整增加买入量
				waitingOrderCnt:= getWaitingSellOrderSize(bot)
				if currentUSDTAmount < bot.LimitMoney  || exchangeCfg.WaitingQueue <= waitingOrderCnt {
					//Printf("[%s][%s %s-USDT]  账户余额不足 :%.4f\n",
					//	TimeNow(),bot.Exchange.GetExchangeName(),bot.Name, currentUSDTAmount)
					time.Sleep(1375 * time.Millisecond)
					continue
				}else {
					//针对卖单队列长度，进行适当调整买入频率
					timeWait:= 1 << uint(waitingOrderCnt)

					time.Sleep(time.Duration(timeWait) * time.Minute)
					continue
				}

				//Println(TimeNow() + "订单完成，如果是卖出订单，可以挂买单")

				currentOrder, cerr := BuyIn(bot.LimitMoney, bot.LimitAmount, latestOrder, bot, exchangeCfg.RoiRate)
				if cerr == nil {
					Printf("[%s] [%s %s-USDT-bot %d] 挂单（买）成功，订单号:%d\n",
						TimeNow(), bot.Exchange.GetExchangeName(), bot.Name, bot.ID, currentOrder.OrderID)
					orderID = currentOrder.OrderID2 //Sprintf("%d", currentOrder.OrderID) //保存最新ID

					//统计当前收益率
					bot.Counter++
					bot.RoiRate += latestOrder.Price / currentOrder.Price //ROI_RATE
					bot.Damping = bot.Damping * 0.9
					//bot.Damping = bot.Damping + float64(time.Now().Unix() - bot.Timestamp) / float64(60 * 60 * 1000)

					//完成时间
					bot.Timestamp = time.Now()

					counterBuyin++
					counterBuyinMoney += currentOrder.Price * currentOrder.Amount

					orderInfo := OrderInfo{currentOrder.Price, currentOrder.Amount,1}
					bot.OrderList[currentOrder.OrderID] = orderInfo //insert one new
				}

			} else if latestOrder.Status == api.ORDER_CANCEL {
				//取消订单了
				Printf("[%s] [%s %s-USDT-bot %d] 订单号:%s 被取消 \n",
					TimeNow(), bot.Exchange.GetExchangeName(), bot.Name, bot.ID, orderID)
				orderID = ""

				if latestOrder.Side == api.SELL {
					counterSellout--
					counterSelloutMoney -= latestOrder.Price * latestOrder.Amount
				}else if latestOrder.Side == api.BUY {
					counterBuyin--
					counterBuyinMoney -= latestOrder.Price * latestOrder.Amount
				}

				//set status to cancel
				item, exist:= bot.OrderList[latestOrder.OrderID]
				if exist == true {
					item.Status = 2
					bot.OrderList[latestOrder.OrderID] = item //modiy
				}

			} else {//订单未完成状态

				//如果长时间(1小时)未成交，且为买入单，尝试取消订单
				//一直未买入成功
				if latestOrder.Side == api.BUY &&
					int(time.Now().Unix()-bot.Timestamp.Unix()) > exchangeCfg.TimeoutBuyOrder {

					shouldCancel, cerr := tryCancelOrder(latestOrder, bot)
					if cerr == nil && shouldCancel == true {
						//需要取消订单，且已经成功
						Printf("[%s] [%s %s-USDT-bot %d] 因长时间未买入成功，取消订单:%s 成功\n",
							TimeNow(), bot.Exchange.GetExchangeName(), bot.Name,bot.ID, orderID)
						orderID = ""
						//set status to cancel
						item, exist:= bot.OrderList[latestOrder.OrderID]
						if exist == true {
							item.Status = 2
							bot.OrderList[latestOrder.OrderID] = item //modiy
						}

					}
				}else if latestOrder.Side == api.SELL &&
					int(time.Now().Unix()-bot.Timestamp.Unix()) > exchangeCfg.TimeoutSellOrder {
					//如果是卖出单，但是长时间未成交，直接忽略该订单
					//一直未卖出成功
					//不取消订单，直接置未空
					Printf("[%s] [%s %s-USDT-bot %d] 订单:%s 长时间未卖出成功，跳出继续买入\n",
						TimeNow(), bot.Exchange.GetExchangeName(), bot.Name, bot.ID, orderID)
					orderID = ""
				}
			}

		} else {
			//第一次进入，直接尝试买入
			waitingOrderCnt:= getWaitingSellOrderSize(bot)
			if currentUSDTAmount < bot.LimitMoney || exchangeCfg.WaitingQueue <= waitingOrderCnt {
				//Printf("[%s]  [%s %s-USDT]账户余额不足 :%.4f\n",
				//	TimeNow(), bot.Exchange.GetExchangeName(),bot.Name,currentUSDTAmount)
				time.Sleep(1237 * time.Millisecond)
				continue
			} else {
				//针对卖单队列长度，进行适当调整买入频率
				timeWait:= 1 << uint(waitingOrderCnt)

				time.Sleep(time.Duration(timeWait) * time.Minute)
				continue
			}
			//Printf("[%s] [%s %s-USDT-bot %d]第一次进入，直接尝试买入\n",
			//	TimeNow(), bot.Exchange.GetExchangeName(), bot.Name, bot.ID)

			var orderTmp *api.Order
			currentOrder, cerr := BuyIn(bot.LimitMoney, bot.LimitAmount, orderTmp, bot, exchangeCfg.RoiRate)
			if cerr == nil {
				Printf("[%s] [%s %s-USDT-bot %d] 挂单（买）成功, 订单号:%s\n",
					TimeNow(), bot.Exchange.GetExchangeName(), bot.Name,bot.ID, currentOrder.OrderID2)
				orderID = currentOrder.OrderID2 //Sprintf("%d", currentOrder.OrderID) //保存最新ID

				//完成时间
				bot.Timestamp = time.Now()

				counterBuyin++
				counterBuyinMoney += currentOrder.Price * currentOrder.Amount

				orderInfo := OrderInfo{currentOrder.Price, currentOrder.Amount,1}
				bot.OrderList[currentOrder.OrderID] = orderInfo //insert one new

			} else {
				if cerr.Error() != "2009" { //2009 余额不足
					Printf("[%s] [%s %s-USDT-bot %d] 第一次进入，买入失败\n",
						TimeNow(), bot.Exchange.GetExchangeName(), bot.Name, bot.ID)
				}
			}
		}

		//TODO 计算收益
		updateTimer++
		if updateTimer >= 5 {// 约1分钟更新一次
			updateStatus(bot)
			updateTimer = 0
		}

	}

	Printf("[%s] [%s %s-USDT-bot %d] bot完成认为，结束\n",
		TimeNow(), bot.Exchange.GetExchangeName(), bot.Name, bot.ID)

}
func getWaitingSellOrderSize(bot *Bot) int{
	counter:=0
	for _, sellOrderID := range bot.OrderPair {
		//检查和更新一遍订单状态，更新成交pair
		order := bot.OrderList[sellOrderID]
		if order.Status == 1 { //只针对未变更的订单（waiting）
			counter++
		}
	}

	return counter
}
func updateStatus(bot *Bot)  {

	for orderid, order := range bot.OrderList {
		//检查和更新一遍订单状态，更新成交pair
		if order.Status == 1 { //只针对未变更的订单（waiting）
			//未完成状态
			strID := Sprintf("%d", orderid)
			orderN ,err:=bot.Exchange.GetOneOrder(strID, bot.CurrencyPair)
			if err ==nil {
				switch orderN.Status {
				case api.ORDER_FINISH:
					//set status to finished
					order.Status = 0 //完成
					bot.OrderList[orderid] = order //modiy order list status
				case api.ORDER_CANCEL:
					order.Status = 2 //cancel
					bot.OrderList[orderid] = order //modiy order list status
				case api.ORDER_UNFINISH:
					order.Status = 1 //waiting
					bot.OrderList[orderid] = order //modiy order list status
				}
			}
			time.Sleep(317 * time.Millisecond)
		}
	}
}
//计算roi
func roiCalculate(bots [10000]Bot, cnt int) (bool) {
	roiRate := 0.0

	roiWell := true //加速度，确定等待时间，是否有收益，决定是否可以启动新的bot

	//很长时间未成交，可以新增
	//成交很快，可以新增

	var timeSpan int64 = 0
	for i := 0; i < cnt; i++ {
		roiRate += bots[i].RoiRate
		timeSpan += (time.Now().Unix() - bots[i].Timestamp.Unix())

	}

	av := float64(timeSpan) / float64(cnt)
	if roiRate >= 0.0 {
		//整体有收益的情况，且平均成单时间小于10分钟，或大于60分钟，经验值
		if av < 600 || av > 3600 {
			roiWell = true
		}
	}
	return roiWell
}

//启动bots
func startBots(bot Bot, exchangeCfg SExchange) {
	//bot start，启动策略
	maxCnt := exchangeCfg.MaxBotNum
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	var bots [10000]Bot //最大启动10000个机器人

	if maxCnt > 10000 {
		maxCnt = 10000
	}

	currBotID := 0

	currCnt := 0

	timer := 0

	printSpan:=0
	var priceBegin float64 = 0.0001
	tickerStart,err:=bot.Exchange.GetTicker(bot.CurrencyPair)
	if err == nil {
		priceBegin = tickerStart.Last
	}
	for systemExit == false {

		//满足一定条件，启动一个新的bot
		var roiWell bool = false
		if timer <= 0 {
			//计算收益率情况, roi, TODO roi calculate
			roiWell = true //roiCalculate(bots, currBotID)

			//只要有收益，就可以启动新的bot
			if roiWell == true && maxCnt > currCnt {
				bots[currBotID] = bot                  //初始化
				bots[currBotID].ID = currBotID + 1     //修改ID
				bots[currBotID].StartTime = time.Now() //启动时间
				bots[currBotID].OrderList = make(map[int] OrderInfo)
				bots[currBotID].OrderPair = make(map[int] int)
				go Start(&bots[currBotID], exchangeCfg)
				currCnt++
				currBotID++
			}

			//设置间隔，最大5*1800s （2.5小时），最少1800s（30分钟）
			timer = exchangeCfg.BotTimeSpan + r.Intn(100)
			Printf("[%s] [%s %s-USDT] random time:%d\n",
				TimeNow(), bot.Exchange.GetExchangeName(), bot.Name, timer)
		}
		timer--
		if printSpan == 600 {//10 min print info

			pairCounter:=0
			waitingOrder:=0
			finishedOrder:=0
			cancelOrder:=0
			finishedPairCounter:=0
			for i:=0;i<currCnt;i++ {
				for _,v:= range bots[i].OrderPair {
					//直接检查卖出单的状态
					if(bots[i].OrderList[v].Status == 0) {
						finishedPairCounter++
					}
				}
				pairCounter += len(bots[i].OrderPair)
				for _,v:= range bots[i].OrderList {
					switch v.Status {
					case 0:
						finishedOrder++
					case 1:
						waitingOrder++
					case 2:
						cancelOrder++
					}
				}
			}
			var priceCurr float64 = 0.0001
			tickerCurr,err:=bot.Exchange.GetTicker(bot.CurrencyPair)
			if err == nil {
				priceCurr = tickerCurr.Last
			}
			Printf("[%s] [%s %s-USDT]总交易对:%d,完成交易对:%d,待成交订单:%d,完成订单:%d,取消订单:%d,总订单:%d,币价从%.4f到%.4f(变化率%.4f%%)\n",
				TimeNow(), bot.Exchange.GetExchangeName(), bot.Name,
					pairCounter, finishedPairCounter, waitingOrder, finishedOrder,cancelOrder,
						(waitingOrder+finishedOrder+cancelOrder),
				priceBegin, priceCurr, 100 * (priceCurr-priceBegin)/priceBegin)

			printSpan = 0
		}
		printSpan++
		time.Sleep(time.Second)

	}

}

//启动一个交易平台
func startExchange(exchange api.API, exchangeCfg SExchange) {

	Printf("[%s] 启动%s bot\n", TimeNow(), exchange.GetExchangeName())

	balanceBeginUSDT:= api.ToFloat64(getBalance(exchange, &api.USDT))//USDT余额
	var balanceBeginCoins float64 = 0

	oldTime := time.Unix(1480390585, 0)
	for _, coin := range exchangeCfg.Coins.Coin {
		if coin.Enable == false {
			continue
		}
		Printf("[%s] %s - [%s-USDT] 启动状态 \n", TimeNow(), exchange.GetExchangeName(), coin.Name)
		pair := api.CurrencyPair{api.Currency{coin.Name, ""}, api.USDT}
		coinBot := Bot{0, exchangeCfg.BuyLimitMoney, coin.LimitAmount,0.0,
			0, oldTime, 0, exchangeCfg.RoiRate, 0,
			pair, exchange, 1.0,
			coin.PriceDecimel, coin.AmountDecimel, coin.Name, time.Now(),
			nil, nil} //初始化
		go startBots(coinBot, exchangeCfg)
		balanceBeginCoins += api.ToFloat64(getBalance(exchange, &api.Currency{coin.Name,""}))
		time.Sleep(time.Second)
	}

	timer:=0
	for systemExit == false { //主线程等待

		if timer  == 120 { //10分钟打印一次
			//获取盈利情况//计算收益
			balanceCurrentUSDT := api.ToFloat64(getBalance(exchange, &api.USDT))

			var balanceCurrentCoins float64 = 0
			coinNames := "USDT"
			for _, coin := range exchangeCfg.Coins.Coin {
				if coin.Enable == false {
					continue
				}
				coinNames = coinNames + "-" + coin.Name
				balanceCurrentCoins += api.ToFloat64(getBalance(exchange, &api.Currency{coin.Name,""}))
			}
			rate := (balanceCurrentUSDT + balanceCurrentCoins - balanceBeginCoins - balanceBeginUSDT) / (balanceBeginCoins + balanceBeginUSDT)
			rate = rate * 100
			Printf("[%s] [%s-USDT]有效货币(%s), 开始余额:%.4f, 当前余额: %.4f，整体累积收益率:%.4f%%\n",
				TimeNow(), exchange.GetExchangeName(), coinNames,
					balanceBeginUSDT + balanceBeginCoins,
				balanceCurrentUSDT + balanceCurrentCoins,
						 rate)
			timer = 0
		}
		timer++

		time.Sleep(5 * time.Second)

	}

}

//交易所监测
func exchangeObserve(exchange api.API, exchangeCfg SExchange) {
	stratage.Start(exchange, exchangeCfg)
}

//获取可用的数字货币数量
func getAvailableAmount(exchange api.API, currency *api.Currency) float64 {
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
func getBalance(exchange api.API, currency *api.Currency) string {
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

//程序退出
func ExitFunc() {

	systemExit = true

	time.Sleep(10 * time.Second)

	os.Exit(0)
}

//入口
func main() {

	configFile := flag.String("c", "../conf/config.xml", "load config file")
	model := flag.String("m","small","exchange model (default in small model)")
	flag.Parse()
	config, err := LoadConfigure(*configFile)

	if err != nil {
		Printf("[%s] 加载配置文件失败，系统正在退出\n", TimeNow())
		return
	}
	//创建监听退出chan
	c := make(chan os.Signal)
	//监听指定信号 ctrl+c kill
	signal.Notify(c, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGUSR1, syscall.SIGUSR2)
	go func() {
		for s := range c {
			switch s {
			case syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
				Printf("[%s] 接收到退出信号，即将退出\n", TimeNow())
				ExitFunc()
				break
			default:
				Printf("[%s] 接收到sys信号，即将退出\n", TimeNow())
				break
			}
			time.Sleep(time.Second)
		}
	}()

	for _, v := range config.Exchanges.Exchange {

		var exchange api.API
		switch strings.ToUpper(v.Name) {
		case "ZB":
			Printf("[%s] zb\n", TimeNow())
			exchange = zb.New(http.DefaultClient,
				v.ApiKey, v.SecretKey)
			break
		case "OKEX":
			Printf("[%s] ok\n", TimeNow())
			exchange = okcoin.NewOKExSpot(http.DefaultClient, v.ApiKey, v.SecretKey)
			break
		default:
			break
		}

		if v.Enable == true {
			switch strings.ToUpper(*model) {
			case "SMALL":
				go startExchange(exchange, v)
				break;
			case "ALLIN":
				go exchangeObserve(exchange, v)
				break;
			}

		} else {
			Printf("[%s] %s not enable\n", TimeNow(), v.Name)
		}
	}

	for systemExit == false { //主线程等待

		time.Sleep(5 * time.Second)

	}

	Printf("[%s] 系统正在退出\n", TimeNow())

}
