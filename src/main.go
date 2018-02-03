package main

import (
	api "./api"
	//"./api/okcoin"
	"./api/zb"
	"errors"
	. "fmt"
	"net/http"
	"time"
	"math/rand"

	"os"
	"os/signal"
	"syscall"
)

var botIDs chan int = make(chan int, 20) //存储botid
var ROI_RATE float64 = 1.003             //千分之3为收益率
var BOT_DEF_AMOUNT float64 = 5           //默认分配给bot的可用金额

var BOT_DEF_CURRENCY api.CurrencyPair = api.XRP_USDT //默认分配给bot的购买比比对

var systemExit bool = false

type Bot struct {
	ID           int              // BotID
	Amount       float64          //当前账户使用金额
	Price        float64          //当前价格
	Type         int              //类型：0，1分别代表买入，卖出
	Timestamp    time.Time            //当前时间
	Status       int              //当前状态：0，1，2分别代表
	RoiRate      float64          //收益率
	Counter      int              //完成个数
	CurrencyPair api.CurrencyPair //
	Exchange     api.API          //
	/*damping = 1/(avr_time + count + 1)*/
	Damping float32 //阻尼系数，表示该bot运行健康度，成交对间隔越短、次数越多，系数越低

	//不同交易所，不同交易货币，精度不一样
	PriceDecimel string //价格精度
	AmountDecimel string //数量精度

	Name string
	StartTime time.Time //启动时间
}


func (bot *Bot) Start() {
	//
	Println("Bot start")
	//1. 检查行情
}

//var zbcom = zb.New(http.DefaultClient, "0fd724ff-5cca-4eb6-acc2-1009ee58d4bc", "fc38cc52-b1d0-4ff6-abb1-4b540763a30e")
//var okexSpot = okcoin.NewOKExSpot(http.DefaultClient, "d5af1693-a715-4c43-8abe-b125dc627f1f", "B1429568E021445F587B953012E53D2F")

func BuyIn(amount float64, latestOrder *api.Order, bot *Bot) (*api.Order, error) {
	retErr := errors.New(TimeNow() + "挂买单失败")

	ticker, err := bot.Exchange.GetTicker(bot.CurrencyPair)
	if err != nil {
		Printf("[%s] [%s %s-USDT] 获取Ticker出错，msg: %s\n",
			TimeNow(),bot.Exchange.GetExchangeName(), bot.Name, err.Error())
		return nil, retErr
	}
	//Printf("买入价格: %.4f\n", ticker.Buy)

	buyPrice := ticker.Buy
	buyAmount := amount / buyPrice
	strbuyAmount := Sprintf(bot.AmountDecimel, buyAmount)
	strBuyPrice := Sprintf(bot.PriceDecimel, buyPrice)
	//xx,_:=strconv.ParseFloat(strBuyPrice, 32)
	//Printf("%.4f\n", xx)

	order, err := bot.Exchange.LimitBuy(strbuyAmount, strBuyPrice, bot.CurrencyPair)
	if nil == err {
		Printf("[%s] [%s %s-USDT] 挂买入单 ok : %d, 价格：%s /%s \n",
			TimeNow(),bot.Exchange.GetExchangeName(), bot.Name, order.OrderID, strBuyPrice, strbuyAmount)
		return order, nil

	} else {
		Printf("[%s] [%s %s-USDT] 挂买入单 err:%s, 价格：%s /%s \n",
			TimeNow(), bot.Exchange.GetExchangeName(), bot.Name, err.Error(),strBuyPrice, strbuyAmount)
	}
	return order, retErr
}
func SellOut(latestOrder *api.Order, bot *Bot , speed int64) (*api.Order, error) {

	retErr := errors.New("挂卖单失败")
	strSellAmount := Sprintf(bot.AmountDecimel, latestOrder.Amount)

	roiRate:=ROI_RATE
	//如果5分钟内成交，可以增大收益率
	if speed < 60 {//1分钟
		roiRate = ROI_RATE * 5
	}else if speed < 120 {//如果2分钟内成交，可以增大收益率
		roiRate = ROI_RATE * 3
	}else if speed < 300 {//5分钟
		roiRate = ROI_RATE * 2
	}else if speed < 600 { //10分钟
		roiRate = ROI_RATE * 1.5
	}else {
		roiRate = ROI_RATE
	}
	sellPrice := latestOrder.Price * roiRate

	ticker, err := bot.Exchange.GetTicker(bot.CurrencyPair)
	if err != nil {
		Printf("[%s] [%s %s-USDT] 挂卖单时获取Ticker出错，message: %s\n",
			TimeNow(),bot.Exchange.GetExchangeName(), bot.Name, err.Error())
		return nil,retErr
	}
	if ticker.Sell > sellPrice { //如果收益计算后比当前市场卖价格低，直接挂市场卖价
		sellPrice = ticker.Sell
	}

	strSellPrice := Sprintf(bot.PriceDecimel, sellPrice)

	order, err := bot.Exchange.LimitSell(strSellAmount, strSellPrice, bot.CurrencyPair)
	if nil == err {
		Printf("[%s] [%s %s-USDT] 挂卖出单 ok : %d，价格:%s / %s \n",
			TimeNow(),bot.Exchange.GetExchangeName(), bot.Name, order.OrderID, strSellPrice, strSellAmount)
		//put sell order
		return order, nil
	} else {
		Printf("[%s] [%s %s-USDT] 挂卖出单err:%s, 价格：%s /%s \n",
			TimeNow(),bot.Exchange.GetExchangeName(), bot.Name, err.Error(), strSellPrice, strSellAmount)
	}
	return order, retErr
}
func tryCancelOrder(latestOrder *api.Order, bot *Bot) (bool, error) {

	shouldCancel := false
	retErr := errors.New("挂取消单失败")
	orderID := latestOrder.OrderID2

	ticker, err := bot.Exchange.GetTicker(bot.CurrencyPair)
	if err != nil {
		Printf("[%s] [%s %s-USDT] 获取Ticker出错，message: %s\n",
			TimeNow(),bot.Exchange.GetExchangeName(), bot.Name, err.Error())
		return shouldCancel, retErr
	}

	if (ticker.Buy / bot.Price) > 0.02 {
		//超过2%，可以取消订单
		Printf("[%s] [%s %s-USDT] 取消订单，买入价格：%.4f, 现价: %.4f\n",
			TimeNow(), bot.Exchange.GetExchangeName(), bot.Name, bot.Price, ticker.Buy)
		shouldCancel = true
		_, err := bot.Exchange.CancelOrder(orderID, bot.CurrencyPair)
		if nil == err {
			//成功
			return shouldCancel, nil
		} else {
			Printf("[%s] [%s %s-USDT] 取消订单失败, err:%s\n",
				TimeNow(),bot.Exchange.GetExchangeName(), bot.Name, err.Error())
		}
	}

	return shouldCancel, retErr

}
func TimeNow() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func Start(bot *Bot) {

	Printf("[%s] [%s %s-USDT] start a bot %d \n",
		TimeNow(),bot.Exchange.GetExchangeName(), bot.Name, bot.ID)

	var orderID string = ""

	var speed int64 =1000000 ////speed 挂卖出单，到卖出单交易成功的时间间隔

	for systemExit == false{

		time.Sleep(10 * time.Second)
		acct, err := bot.Exchange.GetAccount()
		if err != nil {
			Printf("[%s] [%s %s-USDT] bot :%d 获取账户出错，继续， 信息：%s\n",
				TimeNow(), bot.Exchange.GetExchangeName(),bot.Name, bot.ID, err.Error())
			continue
		}

		currentAct := acct.SubAccounts[api.USDT].Amount

		//检查订单状态
		if orderID != "" {
			latestOrder, err := bot.Exchange.GetOneOrder(orderID, bot.CurrencyPair)
			if err != nil {
				Printf("[%s] [%s %s-USDT] 读取订单(%s)状态失败：%s\n",
					TimeNow(), bot.Exchange.GetExchangeName(),bot.Name,orderID, err.Error())
				continue
			}
			/******DUEBUG**
			Printf("orderID:%d (price:%.4f, amount:%.4f, fee:%.4f, status:%d)\n",
				latestOrder.OrderID, latestOrder.Price,latestOrder.Amount,
					latestOrder.Fee, latestOrder.Status)
			*/

			if latestOrder.Status == api.ORDER_FINISH && latestOrder.Side == api.BUY {
				//订单完成，如果是买入订单，则可以挂卖单
				//Println(TimeNow() + "订单完成，如果是买入订单，则可以挂卖单")
				currentOrder, cerr := SellOut(latestOrder, bot, speed)
				if cerr == nil {
					Printf("[%s] [%s %s-USDT] 挂单（卖）订单号： %d\n",
						TimeNow(), bot.Exchange.GetExchangeName(),bot.Name,currentOrder.OrderID)
					orderID = currentOrder.OrderID2//Sprintf("%d", currentOrder.OrderID) //保存最新ID

					//完成时间
					bot.Timestamp = time.Now()

					//TODO TEST 交易完成一次，则退出
					//break
				}

			} else if latestOrder.Status == api.ORDER_FINISH && latestOrder.Side == api.SELL {

				//订单完成，如果是卖出订单，可以挂买单,
				//speed 挂卖出单，到卖出单交易成功的时间间隔
				speed = time.Now().Unix() - bot.Timestamp.Unix()

				//TODO,仓位管理，如果小于80%仓位，不要买入，不能满仓
				//TODO,对于完成很快的bot，适当调整增加买入量
				if currentAct < bot.Amount  {
					//Printf("[%s][%s %s-USDT]  账户余额不足 :%.4f\n",
					//	TimeNow(),bot.Exchange.GetExchangeName(),bot.Name, currentAct)
					continue
				}

				//Println(TimeNow() + "订单完成，如果是卖出订单，可以挂买单")

				currentOrder, cerr := BuyIn(bot.Amount, latestOrder, bot)
				if cerr == nil {
					Printf("[%s] [%s %s-USDT] 挂单（买）成功，订单号：%d\n",
						TimeNow(), bot.Exchange.GetExchangeName(),bot.Name,currentOrder.OrderID)
					orderID = currentOrder.OrderID2//Sprintf("%d", currentOrder.OrderID) //保存最新ID

					//统计当前收益率
					bot.Counter++
					bot.RoiRate += (ROI_RATE - 1)
					bot.Damping = bot.Damping * 0.9
					//bot.Damping = bot.Damping + float64(time.Now().Unix() - bot.Timestamp) / float64(60 * 60 * 1000)

					//完成时间
					bot.Timestamp = time.Now()
				}

			} else if latestOrder.Status == api.ORDER_CANCEL {
				//取消订单了
				Printf("[%s] [%s %s-USDT] 订单号：%s 被取消 \n",
					TimeNow(), bot.Exchange.GetExchangeName(),bot.Name,orderID)
				orderID = ""
			} else {

				//如果长时间(1小时)未成交，且为买入单，尝试取消订单
				if latestOrder.Side == api.BUY &&  (time.Now().Unix() - bot.Timestamp.Unix()) > 3600 {
					shouldCancel, cerr := tryCancelOrder(latestOrder, bot)
					if cerr == nil && shouldCancel == true {
						//需要取消订单，且已经成功
						Printf("[%s] [%s %s-USDT] 取消订单:%s 成功\n",
							TimeNow(), bot.Exchange.GetExchangeName(),bot.Name,orderID)
						orderID = ""

					}
				}
			}

		} else {
			//第一次进入，直接尝试买入

			if currentAct < bot.Amount {
				//Printf("[%s]  [%s %s-USDT]账户余额不足 :%.4f\n",
				//	TimeNow(), bot.Exchange.GetExchangeName(),bot.Name,currentAct)
				continue
			}
			Printf("[%s] [%s %s-USDT]第一次进入，直接尝试买入\n",
				TimeNow(),bot.Exchange.GetExchangeName(),bot.Name)

			var orderTmp *api.Order
			currentOrder, cerr := BuyIn(bot.Amount, orderTmp, bot)
			if cerr == nil {
				Printf("[%s] [%s %s-USDT] 挂单（买）成功, 订单号：%s, %d\n",
					TimeNow(), bot.Exchange.GetExchangeName(),bot.Name, currentOrder.OrderID2, currentOrder.OrderID)
				orderID = currentOrder.OrderID2//Sprintf("%d", currentOrder.OrderID) //保存最新ID

				//完成时间
				bot.Timestamp = time.Now()

			} else {
				Printf("[%s] [%s %s-USDT] 第一次进入，买入失败\n",
					TimeNow(), bot.Exchange.GetExchangeName(),bot.Name)
			}
		}

	}

	Printf("[%s] [%s %s-USDT] bot完成认为，结束\n",
		TimeNow(), bot.Exchange.GetExchangeName(),bot.Name)

}

func roiCalculate(bots [10000]Bot, cnt int) (bool, int) {
	roiRate := 0.0
	counter := 0
	roiWell:=true//加速度，确定等待时间，是否有收益，决定是否可以启动新的bot

	//很长时间未成交，可以新增
	//成交很快，可以新增

	var timeSpan int64 = 0
	for i := 0; i < cnt; i++ {
		roiRate += bots[i].RoiRate
		counter += bots[i].Counter
		timeSpan += (time.Now().Unix() - bots[i].Timestamp.Unix())

	}

	av := float64(timeSpan) / float64(cnt)
	if roiRate >=0.0  {
		//整体有收益的情况，且平均成单时间小于10分钟，或大于60分钟，经验值
		if av < 600 || av > 3600 {
			roiWell = true
		}
	}
	return roiWell, counter
}
func startBots(bot Bot, maxCnt int)  {
	//bot start，启动策略
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	var bots [10000]Bot //最大启动10000个机器人

	if maxCnt > 10000 {
		maxCnt = 10000
	}

	currBotID := 0

	currCnt:=0

	for systemExit == false{

		//满足一定条件，启动一个新的bot

		//计算收益率情况, roi
		roiWell, counter:=roiCalculate(bots, currBotID)

		//只要有收益，就可以启动新的bot
		if roiWell==true && maxCnt > currCnt {
			bots[currBotID] = bot //初始化
			bots[currBotID].ID = currBotID + 1 //修改ID
			bots[currBotID].StartTime = time.Now() //启动时间
			go Start(&bots[currBotID])
			currCnt++
		}

		//设置间隔，10分钟
		span := time.Duration( 10 * time.Minute + time.Duration(r.Intn(1000000)))
		time.Sleep(span)


		Printf("[%s] [%s %s-USDT] 累积成交对：%d\n",
			TimeNow(),bot.Exchange.GetExchangeName(), bot.Name, counter)
	}

}

func zbExchange(exchange api.API)  {

	/////////// zb bot ////////////

	oldTime:=time.Unix(1480390585, 0)
	maxBotCnt := 10

	//HSR bot
	hsrBot :=  Bot{0, BOT_DEF_AMOUNT, 0.0,
		0, oldTime, 0, 0.0, 0,
		api.HSR_USDT, exchange, 1.0,
		"%.2f", "%.2f", "HSR", time.Now()} //初始化
	go startBots(hsrBot, maxBotCnt)

	time.Sleep(time.Second)

	//BTC bot
	btcBot :=  Bot{0, BOT_DEF_AMOUNT, 0.0,
		0, oldTime, 0, 0.0, 0,
		api.BTC_USDT, exchange, 1.0,
		"%.2f", "%.4f", "BTC",time.Now()} //初始化
	go startBots(btcBot, maxBotCnt)

	time.Sleep(time.Second)


	//ETH bot
	ethBot :=  Bot{0, BOT_DEF_AMOUNT, 0.0,
		0, oldTime, 0, 0.0, 0,
		api.ETH_USDT, exchange, 1.0,
		"%.2f", "%.3f", "ETH", time.Now()} //初始化
	go startBots(ethBot, maxBotCnt)
	time.Sleep(time.Second)

	/*
	//ETC bot
	etcBot :=  Bot{0, BOT_DEF_AMOUNT, 0.0,
		0, oldTime, 0, 0.0, 0,
		api.ETC_USDT, exchange, 1.0,
		"%.2f", "%.2f", "ETC",time.Now()} //初始化
	go startBots(etcBot, maxBotCnt)
	time.Sleep(time.Second)
	*/
	//BCC bot
	bccBot :=  Bot{0, BOT_DEF_AMOUNT, 0.0,
		0, oldTime, 0, 0.0, 0,
		api.BCC_USDT, exchange, 1.0,
		"%.2f", "%.3f", "BCC",time.Now()} //初始化
	go startBots(bccBot, maxBotCnt)
	time.Sleep(time.Second)
	/*
	//LTC bot
	ltcBot :=  Bot{0, BOT_DEF_AMOUNT, 0.0,
		0, oldTime, 0, 0.0, 0,
		api.LTC_USDT, exchange, 1.0,
		"%.2f", "%.3f", "LTC",time.Now()} //初始化
	go startBots(ltcBot, maxBotCnt)
	*/

}
/*
func okExchange()  {


	//////////OK Bot///////////////
	var exchange = okcoin.NewOKExSpot(http.DefaultClient,
		"d5af1693-a715-4c43-8abe-b125dc627f1f", "B1429568E021445F587B953012E53D2F")
	acctBegin, err := exchange.GetAccount()
	if err != nil {
		Printf("[%s] 获取账户出错，系统退出，错误信息：%s\n", TimeNow(), err.Error())
		return
	}
	beginAct := acctBegin.SubAccounts[api.USDT].Amount

	count := 1
	var bots [20]Bot

	//bot start，启动策略
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < count; i++ {
		bots[i] = Bot{i + 1, BOT_DEF_AMOUNT, 0.0,
			0, 0, 0, 0.0, 0,
			api.HSR_USDT, exchange, 1.0,
			"%.2f", "%.2f"} //初始化
		go Start(&bots[i])

		//设置bot启动间隔，10分钟启动一个
		time.Sleep(10 * time.Minute + r.Intn(1000000))
	}


	for { //祝线程等待
		time.Sleep(time.Minute)

		//计算收益率情况
		roiRate := 0.0
		counter := 0
		for i := 0; i < count; i++ {
			roiRate += bots[i].RoiRate
			counter += bots[i].Counter
		}

		acct, err := exchange.GetAccount()

		if err != nil {
			Printf("[%s] 主程序获取账户出错，继续，错误信息：%s\n", TimeNow(), err.Error())
			continue
		}
		currentAct := acct.SubAccounts[api.USDT].Amount
		Printf("[%s] USDT 开始余额: %.4f, 当前余额: %.4f，累积成交对：%d，累积收益率：%.4f\n",
			TimeNow(), beginAct, currentAct, counter, roiRate)

		//Printf("当前账户余额:%.4f, %.4f，收益：%.4f\n",acct.Asset,acct.NetAsset, acct.Asset - acctBegin.Asset)

	}

}
*/
func getBalance(exchange api.API, currency *api.Currency) string {
	acc, err := exchange.GetAccount()
	if err != nil {
		//error
		Printf("[%s] getBalance error , err message: %s\n", TimeNow(), err.Error())
		return "0.0000"
	}
	balance := 0.0
	for curr, subItem := range acc.SubAccounts {
		if currency != nil {

			if curr == *currency {
				amount := subItem.Amount + subItem.ForzenAmount

				if amount > 0 {
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
		}else {//get full
			amount := subItem.Amount + subItem.ForzenAmount

			if amount > 0  && curr != api.USDT {
				ticker, err := exchange.GetTicker(api.CurrencyPair{curr, api.USDT})
				if err != nil {
					Printf("[%s] getBalance of %s error of get ticker , err message: %s\n",
						TimeNow(), curr.String(), err.Error())
					continue//忽略掉
				}
				//Printf("%s,%.4f\n",curr.String(), amount)
				balance += amount * ticker.Last
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	return Sprintf("%.4f", balance)

}
func ExitFunc()  {

	systemExit = true

	time.Sleep(10 * time.Second)

	os.Exit(0)
}
func main() {

	
	//创建监听退出chan
	c := make(chan os.Signal)
	//监听指定信号 ctrl+c kill
	signal.Notify(c, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGUSR1, syscall.SIGUSR2)
	go func() {
		for s := range c {
			switch s {
			case syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
				Println("系统退出", s)
				ExitFunc()
			case syscall.SIGUSR1:
				Println("usr1", s)
			case syscall.SIGUSR2:
				Println("usr2", s)
			default:
				Println("other", s)
			}
			time.Sleep(time.Second)
		}
	}()

	var zbexchange = zb.New(http.DefaultClient,
		"0fd724ff-5cca-4eb6-acc2-1009ee58d4bc", "fc38cc52-b1d0-4ff6-abb1-4b540763a30e")


	zbbalanceBegin := getBalance(zbexchange, nil)
	zbExchange(zbexchange)
	//Println("xxx")
	for systemExit == false { //主线程等待

		//获取盈利情况
		balanceNow := getBalance(zbexchange, nil)
		rate:= (api.ToFloat64(balanceNow) - api.ToFloat64(zbbalanceBegin)) / api.ToFloat64(zbbalanceBegin)
		rate = rate * 100
		Printf("[%s] [zb-USDT] 开始余额：%s, 当前余额: %s，整体累积收益率：%.4f %%\n",
			TimeNow(), zbbalanceBegin, balanceNow, rate)

		time.Sleep(10 * time.Minute)

	}


}
