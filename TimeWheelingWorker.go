package dxserver

import(
	"time"
	"sync"
)

type(
	TimeWheelWorker struct {
		sync.Mutex					//调度锁
		ticker			*time.Ticker		//调度器时钟
		timeslocks		[]chan struct{}		//时间槽
		slockcount		int
		maxTimeout 		time.Duration
		quitchan		chan struct{}
		curindex		int			//当前的索引
		interval		time.Duration
	}
)

var(
	defaultTimeWheelWorker   *TimeWheelWorker
)


//interval指定调度的时间间隔
//slotBlockCount指定时间轮的块长度
func NewTimeWheelWorker(interval time.Duration,slotBlockCount int) *TimeWheelWorker {
	result := new(TimeWheelWorker)
	result.interval = interval
	result.quitchan = make(chan struct{})
	result.slockcount = slotBlockCount
	result.maxTimeout = interval * time.Duration(slotBlockCount)
	result.timeslocks = make([]chan struct{},slotBlockCount)
	result.ticker = time.NewTicker(interval)
	go result.run()
	return result
}

func (worker *TimeWheelWorker)run()  {
	for{
		select {
		case <-worker.ticker.C:
			//执行定时操作
			//获取当前的时间槽数据
			worker.Lock()
			lastC := worker.timeslocks[worker.curindex]
			worker.timeslocks[worker.curindex] = make(chan struct{})
			worker.curindex = (worker.curindex + 1) % worker.slockcount
			worker.Unlock()
			close(lastC)
		case <-worker.quitchan:
			worker.ticker.Stop()
			return
		}
	}
}


func (worker *TimeWheelWorker)Stop()  {
	close(worker.quitchan)
}

func (worker *TimeWheelWorker)After(d time.Duration)<-chan struct{}  {
	if d >= worker.maxTimeout {
		panic("timeout too much, over maxtimeout")
	}
	index := int(d / worker.interval)
	if index > 0 {
		index--
	}
	worker.Lock()
	index = (worker.curindex + index) % worker.slockcount
	b := worker.timeslocks[index]
	if b == nil{
		b = make(chan struct{})
		worker.timeslocks[index] = b
	}
	worker.Unlock()
	return b
}

func (worker *TimeWheelWorker)Sleep(d time.Duration)  {
	<-worker.After(d)
}


func After(d time.Duration)<-chan struct{}  {
	if defaultTimeWheelWorker == nil{
		defaultTimeWheelWorker = NewTimeWheelWorker(time.Millisecond*500,7200)
	}
	return defaultTimeWheelWorker.After(d)
}


func Sleep(d time.Duration)  {
	if defaultTimeWheelWorker == nil{
		defaultTimeWheelWorker = NewTimeWheelWorker(time.Millisecond*500,7200)
	}
	defaultTimeWheelWorker.Sleep(d)
}