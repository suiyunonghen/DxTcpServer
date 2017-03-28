//时间轮工作调度
//作者：不得闲
//QQ：75492895
package dxserver

import(
	"time"
	"sync"
	"container/list"
	"math"
)

type(

	//时间槽中的对象节点
	timeSlockNode struct {
		runindex		int			//第几圈的时候执行
		slockchan		chan struct{}
		runfunc			func()			//执行的方法
		interval		time.Duration		//执行间隔
	}
	TimeWheelWorker struct {
		sync.RWMutex					//调度锁
		ticker			*time.Ticker		//调度器时钟
		timeslocks		[]*list.List		//时间槽
		slockcount		int
		quitchan		chan struct{}
		curindex		int			//当前的索引
		interval		time.Duration
	}
)

var(
	defaultTimeWheelWorker   *TimeWheelWorker
)

func (node *timeSlockNode)calcNextRunIndex(schedinterval time.Duration,slotlen,curindex int)int  {
	if node.interval == 0{
		node.runindex = -1
		return  -1
	}
	idx := int(node.interval / schedinterval)
	if node.interval % schedinterval == 0 && idx > 0{
		idx--
	}
	node.runindex = idx / slotlen			//执行第几圈
	idx = (idx + curindex + 1) % slotlen //实际在时间槽中的位置
	return idx
}

//interval指定调度的时间间隔
//slotBlockCount指定时间轮的块长度
func NewTimeWheelWorker(interval time.Duration,slotBlockCount int) *TimeWheelWorker {
	result := new(TimeWheelWorker)
	result.interval = interval
	result.quitchan = make(chan struct{})
	result.slockcount = slotBlockCount
	result.timeslocks = make([]*list.List,slotBlockCount)
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
			worker.RLock()
			timeslock := worker.timeslocks[worker.curindex]
			worker.RUnlock()
			var (
				node *timeSlockNode
				tmplist *list.List
			)
			if timeslock != nil && timeslock.Len()>0{
				ele := timeslock.Front()
				backele := ele
				for {
					node = ele.Value.(*timeSlockNode)
					if node.runindex <= worker.curindex{
						//执行并且移除重新计算位置
						if node.slockchan!=nil{
							close(node.slockchan) //关闭然后自动广播
							node.slockchan = nil
						}
						if node.runfunc!=nil{
							go node.runfunc()
						}
						backele = ele
						ele = ele.Next()
						//计算下一个循环的处理结构
						worker.Lock()
						posidx := node.calcNextRunIndex(worker.interval,worker.slockcount,worker.curindex)
						if posidx != worker.curindex || node.runindex != worker.curindex{
							//移除
							backele.Value = nil
							timeslock.Remove(backele)
							if posidx != -1{ //重建位置
								tmplist = worker.timeslocks[posidx]
								if tmplist == nil{
									tmplist = list.New()
									worker.timeslocks[posidx] = tmplist
								}
								tmplist.PushBack(node)
							}
						}
						worker.Unlock()
						if ele == nil{
							break
						}
					}
				}
			}
			worker.curindex++
		case <-worker.quitchan:
			worker.ticker.Stop()
			close(worker.quitchan)
			return
		default:
			//执行一次重置CurIndex
			if worker.curindex == math.MaxInt32 - 5000{
				worker.ReSetCurindex()
			}
		}
	}
}

//重置轮渡顺序
func (worker *TimeWheelWorker)ReSetCurindex()  {
	var (
		node *timeSlockNode
		backele,ele *list.Element
		tmplist	*list.List
	)
	worker.Lock()
	for index,lst := range worker.timeslocks{
		if lst != nil && lst.Len()>0{
			ele = lst.Front()
			for {
				backele = ele
				node = ele.Value.(*timeSlockNode)
				node.runindex = node.runindex - worker.curindex
				if node.runindex < 0 && index == worker.curindex{
					//立即执行
					if node.slockchan!=nil{
						close(node.slockchan) //关闭然后自动广播
						node.slockchan = nil
					}
					if node.runfunc!=nil{
						go node.runfunc()
					}
					posidx := int(node.interval / worker.interval)
					if node.interval % worker.interval == 0 && posidx > 0{
						posidx--
					}
					node.runindex = posidx / worker.slockcount		//执行第几圈
					posidx = (posidx + 1) % worker.slockcount 		//实际在时间槽中的位置
					if posidx != worker.curindex || node.runindex != worker.curindex{
						//移除
						backele.Value = nil
						lst.Remove(backele)
						if posidx != -1{ //重建位置
							tmplist = worker.timeslocks[posidx]
							if tmplist == nil{
								tmplist = list.New()
								worker.timeslocks[posidx] = tmplist
							}
							tmplist.PushBack(node)
						}
					}
				}
				ele = ele.Next()
				if ele == nil{
					break
				}
			}
		}
	}
	worker.curindex = 0
	worker.Unlock()
}

func (worker *TimeWheelWorker)Stop()  {
	worker.quitchan<- struct{}{}
}

func (worker *TimeWheelWorker)After(d time.Duration)<-chan struct{}  {
	idx := int(d / worker.interval)
	if d % worker.interval == 0 && idx > 0{
		idx--
	}
	worker.Lock()
	runidx := idx / worker.slockcount + worker.curindex	//执行第几圈
	idx = (idx + worker.curindex) % worker.slockcount 	//实际在时间槽中的位置
	slockList := worker.timeslocks[idx]
	if slockList == nil{
		slockList = list.New()
		worker.timeslocks[idx] = slockList
	}else{
		//查找是否有相同的时间圈和时间槽的
		var nd *timeSlockNode
		ele := slockList.Front()
		for ele != nil{
			nd = ele.Value.(*timeSlockNode)
			if nd.runindex == runidx{
				if nd.slockchan == nil{
					nd.slockchan = make(chan struct{})
				}
				worker.Unlock()
				return nd.slockchan
			}
			ele = ele.Next()
		}
	}
	node := new(timeSlockNode)
	node.interval = 0
	node.runfunc = nil
	node.slockchan = make(chan struct{})
	node.runindex = runidx
	slockList.PushBack(node)
	worker.Unlock()
	return node.slockchan
}

func (worker *TimeWheelWorker)AfterFunc(d time.Duration,fun func())  {
	idx := int(d / worker.interval)
	if d % worker.interval == 0 && idx > 0{
		idx--
	}
	worker.Lock()
	runidx := idx / worker.slockcount + worker.curindex	//执行第几圈
	idx = (idx + worker.curindex) % worker.slockcount 	//实际在时间槽中的位置
	slockList := worker.timeslocks[idx]
	if slockList == nil{
		slockList = list.New()
		worker.timeslocks[idx] = slockList
	}else{
		//查找是否有相同的时间圈和时间槽的
		var nd *timeSlockNode
		ele := slockList.Front()
		for ele != nil{
			nd = ele.Value.(*timeSlockNode)
			if nd.runindex == runidx{
				if nd.runfunc == nil{
					nd.runfunc = fun
					worker.Unlock()
					return
				}
			}
			ele = ele.Next()
		}
	}
	node := new(timeSlockNode)
	node.interval = 0
	node.runfunc = fun
	node.runindex = runidx
	slockList.PushBack(node)
	worker.Unlock()
}

func (worker *TimeWheelWorker)Sleep(d time.Duration)  {
	<-worker.After(d)
}

//间隔执行
func (worker *TimeWheelWorker)TickFunc(d time.Duration,fun func())  {
	idx := int(d / worker.interval)
	if d % worker.interval == 0 && idx > 0{
		idx--
	}
	worker.Lock()
	runidx := idx / worker.slockcount + worker.curindex	//执行第几圈
	idx = (idx + worker.curindex) % worker.slockcount 	//实际在时间槽中的位置
	slockList := worker.timeslocks[idx]
	if slockList == nil{
		slockList = list.New()
		worker.timeslocks[idx] = slockList
	}else{
		//查找是否有相同的时间圈和时间槽的
		var nd *timeSlockNode
		ele := slockList.Front()
		for ele != nil{
			nd = ele.Value.(*timeSlockNode)
			if nd.runindex == runidx{
				if nd.runfunc == nil{
					nd.interval = d
					nd.runfunc = fun
					worker.Unlock()
					return
				}
			}
			ele = ele.Next()
		}
	}
	node := new(timeSlockNode)
	node.runfunc = fun
	node.interval = d
	node.runindex = runidx
	slockList.PushBack(node)
	worker.Unlock()
}

func (worker *TimeWheelWorker)ReSet()  {
	worker.Stop()
	worker.ReSetCurindex()
	worker.quitchan = make(chan struct{})
	worker.ticker = time.NewTicker(worker.interval)
	go worker.run()
}

func After(d time.Duration)<-chan struct{}  {
	if defaultTimeWheelWorker == nil{
		defaultTimeWheelWorker = NewTimeWheelWorker(time.Second,3600) //1秒钟一次
	}
	return defaultTimeWheelWorker.After(d)
}

func AfterFunc(d time.Duration,fun func())  {
	if defaultTimeWheelWorker == nil{
		defaultTimeWheelWorker = NewTimeWheelWorker(time.Second,3600) //1秒钟一次
	}
	defaultTimeWheelWorker.AfterFunc(d,fun)
}

func Sleep(d time.Duration)  {
	if defaultTimeWheelWorker == nil{
		defaultTimeWheelWorker = NewTimeWheelWorker(time.Second,3600) //1秒钟一次
	}
	defaultTimeWheelWorker.Sleep(d)
}

func TickFunc(d time.Duration,fun func())  {
	if defaultTimeWheelWorker == nil{
		defaultTimeWheelWorker = NewTimeWheelWorker(time.Second,3600) //1秒钟一次
	}
	defaultTimeWheelWorker.TickFunc(d,fun)
}


