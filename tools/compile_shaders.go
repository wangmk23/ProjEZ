//go:build ignore
// Run with go run tools/compile_shaders.go from source. Uses system D3DCompiler_47 only at build time.
package main
import("fmt";"os";"syscall";"unsafe")
type blob struct{v *[5]uintptr}
func call(b *blob,i int)uintptr{r,_,_:=syscall.SyscallN(b.v[i],uintptr(unsafe.Pointer(b)));return r}
func main(){src,e:=os.ReadFile("shaders/projection.hlsl");if e!=nil{panic(e)}
 p:=syscall.NewLazyDLL("d3dcompiler_47.dll").NewProc("D3DCompile")
 for _,s:=range []struct{entry,target,path string}{{"VS","vs_4_0","assets/projection_vs.cso"},{"PS","ps_4_0","assets/projection_ps.cso"}} {
  ep,_:=syscall.BytePtrFromString(s.entry);tp,_:=syscall.BytePtrFromString(s.target);var out,errBlob *blob
  hr,_,_:=p.Call(uintptr(unsafe.Pointer(&src[0])),uintptr(len(src)),0,0,0,uintptr(unsafe.Pointer(ep)),uintptr(unsafe.Pointer(tp)),1<<15,0,uintptr(unsafe.Pointer(&out)),uintptr(unsafe.Pointer(&errBlob)))
  if errBlob!=nil{fmt.Println(string(unsafe.Slice((*byte)(unsafe.Pointer(call(errBlob,3))),call(errBlob,4))));call(errBlob,2)}
  if int32(hr)<0{panic(fmt.Sprintf("D3DCompile %08x",hr))}
  data:=unsafe.Slice((*byte)(unsafe.Pointer(call(out,3))),call(out,4));e=os.WriteFile(s.path,data,0644);call(out,2);if e!=nil{panic(e)};fmt.Println(s.path,len(data))
 }
}
