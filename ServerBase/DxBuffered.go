package ServerBase
import (
	"io"
	"sync"
)

type DxReader struct {
	chainbuf     [][]byte
	rd           io.Reader
	r, w         int
	ridx,widx	 int
	err          error
	bufsize		 int
	lastByte     int
	lastRuneSize int
	bytepool	 sync.Pool
}

func (r *DxReader)MarkIndex()(int, int)  {
	return r.ridx,r.r
}

func (r *DxReader)RestoreMark(chainIndex,roffset int)  {
	if chainIndex <= r.widx{
		r.ridx = chainIndex
		if r.r <= r.w{
			r.r = roffset
		}
	}
}

func (r *DxReader)ClearRead()  {
	if len(r.chainbuf)>0{
		for i := 0;i<r.ridx;i++{
			r.bytepool.Put(r.chainbuf[i])
		}
		if r.ridx>0{
			copy(r.chainbuf,r.chainbuf[r.ridx:])
		}
		r.widx -= r.ridx
		if r.widx > 0{
			r.chainbuf = r.chainbuf[:r.widx]
		}
		r.ridx = 0
	}
}


//读取附加
func (r *DxReader)ReadAppend()(rlen int,e error,canNextRead bool)  {
	//是否读取到数据了
	var buf []byte=nil
	if len(r.chainbuf)>0{
		buf = r.chainbuf[r.widx]
		if len(buf)==r.widx{
			buf = nil
		}
	}
	if buf == nil{
		v := r.bytepool.Get()
		if v == nil {
			v = make([]byte, r.bufsize)
		}
		buf = v.([]byte)
		r.chainbuf = append(r.chainbuf,buf)
		r.widx += 1
		r.w = 0
	}
	buflen := r.bufsize-r.w //剩下的长度
	rlen, e = r.rd.Read(buf[r.w:])
	r.w += rlen
	canNextRead = rlen < buflen //是否可以读取下一波
	return
}

//缓冲区的数据长度
func (r *DxReader)Buffered()int  {
	if r.ridx == r.widx{
		return r.w-r.r
	}
	result := len(r.chainbuf[r.ridx])-r.r
	for i := r.ridx+1;i<r.widx-1;i++{
		result += len(r.chainbuf[i])
	}
	return result+r.w
}

//读取数据
func (reader *DxReader)Read(p []byte) (int, error)  {
	if reader.IsEmpty(){
		return reader.rd.Read(p)
	}
	wrlen := len(p)
	buf := reader.chainbuf[reader.ridx]
	rlen := reader.bufsize-reader.r
	if rlen >= wrlen{
		//直接读完
		copy(p,buf[reader.r:reader.r+wrlen])
		reader.r+=wrlen
		return wrlen,nil
	}
	copy(p,buf[reader.r:])
	wrlen -= rlen
	for i := reader.ridx + 1;i < reader.widx;i++{
		reader.ridx += 1
		reader.r = 0
		buf = reader.chainbuf[i]
		if reader.bufsize >= wrlen{
			copy(p[rlen:],buf[:wrlen])
			reader.r+=wrlen
			return len(p),nil
		}
		copy(p[rlen:],buf)
		rlen += reader.bufsize
		wrlen -= reader.bufsize
	}
	if reader.widx != reader.ridx{
		buf = reader.chainbuf[reader.widx]
		if reader.w >= wrlen{
			copy(p[rlen:],buf[:wrlen])
			return len(p),nil
		}
		copy(p[rlen:],buf[:reader.w])
		rlen += reader.w
		wrlen -= reader.w
	}
	reader.ClearRead()
	rl,err := reader.rd.Read(p[rlen:])
	return rl+rlen,err
}

func (reader *DxReader)IsEmpty()bool  {
	return len(reader.chainbuf)==0 || reader.r==reader.w && reader.ridx==reader.widx
}

func (reader *DxReader)TotalSize()int  {
	return len(reader.chainbuf)*reader.bufsize
}

func NewDxReader(rd io.Reader, bufersize int)*DxReader  {
	result := new(DxReader)
	result.rd = rd
	result.widx = -1
	result.chainbuf = make([][]byte,0,20)
	result.bufsize = bufersize
	return result
}