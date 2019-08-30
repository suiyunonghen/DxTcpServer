package RPC

import (
	"testing"
	"fmt"
	"reflect"
)

type Mystruct struct{

}

func (m *Mystruct)Test(x,y int)  {
	fmt.Println(x*y)
}

func (m Mystruct)Test1(x,y int)  {
	fmt.Println(x+y)
}

func max(x,y int)int  {
	return x+y
}


func Test_reflect(t *testing.T)  {
	mt := reflect.TypeOf(max)
	fmt.Println(mt.Kind())
	fmt.Println(mt.Name())
	mt = reflect.TypeOf(&Mystruct{})
	fmt.Println(mt.Kind())
	fmt.Println(mt.Name())
	fmt.Println(mt.NumMethod())
	mv := make([]reflect.Value,3)
	mv[0] = reflect.ValueOf(&Mystruct{})
	mv[1] = reflect.ValueOf(3)
	mv[2] = reflect.ValueOf(4)
	for i := 0;i<mt.NumMethod();i++{
		mnd := mt.Method(i)
		mnd.Func.Call(mv)
		fmt.Println(mnd.Name)
		fmt.Println(mnd.Func.Type().NumIn())

	}
}
