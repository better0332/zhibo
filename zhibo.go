package main

/*
	#include "live.h"
*/
import "C"

import (
	"flag"
	"io"
	"net/http"
	"net/url"
	"runtime"

	"github.com/golang/glog"
	// rdebug "runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"zhibo/db"
	"zhibo/flv"
	"zhibo/queue"
)

type stat struct {
	// must operation by "sync/atomic"
	online      int64 // cate == 1
	cacheSize   int64
	serviceSize int64
	isExit      int64
	isSync      int64

	cate uint8

	ipLock sync.Mutex // cate == 0
	ipMap  map[string]int
}

var (
	cachedUrl = struct {
		sync.Mutex
		m map[string]*queue.RespQueue
		a []string
	}{m: make(map[string]*queue.RespQueue)}

	cachedStream = struct {
		sync.Mutex
		m map[string]*queue.StreamQueue
	}{m: make(map[string]*queue.StreamQueue)}

	liveIdMap = struct {
		sync.RWMutex
		m map[int]*stat
	}{m: make(map[int]*stat)}

	maxMem = 100 << 20 // 100MB
)

func initLiveId() {
	rows, err := db.QueryLiveId()
	if err != nil {
		glog.V(0).Infof("initLiveId error: %v\n", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var live_id int
		var category uint8
		var cache_size, service_size int64
		if rows.Scan(&live_id, &category, &cache_size, &service_size) == nil {
			glog.V(1).Infof("Init LiveId: %d\n", live_id)
			liveIdMap.m[live_id] = &stat{
				cacheSize:   cache_size,
				serviceSize: service_size,
				isSync:      1,
				cate:        category,
				ipMap:       make(map[string]int)}
		}
	}
}

func zhiboGC() {
	for {
		time.Sleep(30 * time.Second)

		glog.V(2).Infoln("start gc...")
		cachedUrl.Lock()

		delMem := getTotalSize() - maxMem
		save := delMem
		glog.V(2).Infoln("delMem: ", delMem)
		for delMem > 0 {
			uri := cachedUrl.a[0]
			glog.V(2).Infoln("[delete uri] ", uri)
			cachedUrl.a = cachedUrl.a[1:]
			if q, ok := cachedUrl.m[uri]; ok {
				delMem -= q.GetSize()
				q.Del()
				delete(cachedUrl.m, uri)
			}
		}
		if save > 10<<20 {
			runtime.GC()
			// rdebug.FreeOSMemory()
		}

		cachedUrl.Unlock()
		glog.V(2).Infoln("end gc")
	}
}

func updateStat() {
	for {
		time.Sleep(60 * time.Second)

		glog.V(2).Infoln("updateStat")

		save := make(map[int]*stat)

		liveIdMap.RLock()
		for liveId, st := range liveIdMap.m {
			save[liveId] = st
		}
		liveIdMap.RUnlock()

		for liveId, st := range save {
			if atomic.LoadInt64(&st.isSync) == 0 {
				continue
			}
			var online int64
			if st.cate == 0 {
				st.ipLock.Lock()
				online = int64(len(st.ipMap))
				st.ipMap = make(map[string]int, online)
				st.ipLock.Unlock()
			} else if st.cate == 1 {
				online = atomic.LoadInt64(&st.online)
			}
			db.Update(liveId, online,
				atomic.LoadInt64(&st.cacheSize),
				atomic.LoadInt64(&st.serviceSize))
		}
	}
}

func recvCommand() {
	for {
		var live_id C.uint32_t
		var command C.uint8_t
		var category C.uint8_t
		C.read_live_info_from_IPC(&live_id, &command, &category)

		if command == 0 { // stop live
			if live_id != 0 {
				glog.V(1).Infof("live_id=%d stop live\n", live_id)
				if st, ok := liveIdMap.m[int(live_id)]; ok {
					atomic.StoreInt64(&st.isExit, 1)
					liveIdMap.Lock()
					delete(liveIdMap.m, int(live_id))
					liveIdMap.Unlock()
				}
			} else {
				glog.V(1).Infof("stop all live")
				for _, st := range liveIdMap.m {
					atomic.StoreInt64(&st.isExit, 1)
				}
				liveIdMap.Lock()
				liveIdMap.m = make(map[int]*stat)
				liveIdMap.Unlock()
			}
		} else if command == 1 { // start live
			glog.V(1).Infof("live_id=%d start live\n", live_id)
			st := &stat{cate: uint8(category), ipMap: make(map[string]int)}
			liveIdMap.Lock()
			liveIdMap.m[int(live_id)] = st
			liveIdMap.Unlock()

			time.AfterFunc(60*time.Second, func() {
				atomic.StoreInt64(&st.isSync, 1)
			})
		}
	}
}

//not lock cachedUrl
func getTotalSize() int {
	size := 0
	for _, q := range cachedUrl.m {
		size += q.GetSize()
	}
	return size
}

func crossdomain(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "/home/nginx/html/crossdomain.xml")
}

func sendRedir(w http.ResponseWriter, u *url.URL) {
	urlStr := u.String()
	if strings.IndexByte(urlStr, '?') > 0 {
		urlStr += "&redir="
	} else {
		urlStr += "?redir="
	}

	w.Header().Set("Location", urlStr)
	w.WriteHeader(302)
}

func zhiboServer(w http.ResponseWriter, r *http.Request) {
	pos := strings.IndexByte(r.RequestURI[1:], '/')
	if pos < 1 {
		glog.V(0).Infof("%s: invalid liveid\n", r.RequestURI)
		return
	}
	liveId, err := strconv.Atoi(r.RequestURI[1 : pos+1])
	if err != nil {
		glog.V(0).Infof("%s: %v\n", r.RequestURI, err)
		return
	}
	category := r.RequestURI[pos+2]

	var cookie string
	uri := "http://" + r.RequestURI[pos+4:]
	if pos = strings.Index(uri, "proxy_cookie="); pos > 0 {
		cookie, _ = url.QueryUnescape(uri[pos+13:])
		uri = uri[:pos]
	}
	if r.URL, err = url.Parse(uri); err != nil {
		glog.V(0).Infof("%s: %v\n", uri, err)
		return
	}
	if cookie != "" {
		r.Header.Set("Cookie", cookie)
	}
	// if pos = strings.IndexByte(uri, '?'); pos > 0 {
	// 	uri = uri[:pos]
	// }
	uri = r.URL.Path

	liveIdMap.RLock()
	st, ok := liveIdMap.m[liveId]
	liveIdMap.RUnlock()
	if !ok {
		glog.V(0).Infof("%s: invalid liveid\n", r.RequestURI)
		sendRedir(w, r.URL)
		return
	}

	r.Host = r.URL.Host
	r.RequestURI = ""

	if category == '0' {
		cachedUrl.Lock()
		q, ok := cachedUrl.m[uri]
		if !ok {
			q = queue.NewRespQueue()
			cachedUrl.m[uri] = q
			cachedUrl.a = append(cachedUrl.a, uri)
			cachedUrl.Unlock()

			glog.V(1).Infof("start getting %s [%s]\n", uri, r.RemoteAddr)

			resp, err := http.DefaultClient.Do(r)
			if err != nil || resp.StatusCode != 200 {
				q.PushResp(nil)
				// 如果删除，那么后面的client可能会重新请求该uri，否则后面的client将永远重定向该uri
				cachedUrl.Lock()
				delete(cachedUrl.m, uri) //delete cachedUrl.a hard
				cachedUrl.Unlock()

				if err == nil {
					resp.Body.Close()
				}
				return
			}
			defer resp.Body.Close()

			q.PushResp(resp)

			go func() {
				for {
					buf := make([]byte, 16<<10) // 16KB
					n, err := io.ReadFull(resp.Body, buf)
					if n > 0 {
						atomic.AddInt64(&st.cacheSize, int64(n))
						if !q.Push(buf[:n]) {
							// this q has deleted by gc
							break
						}
					}
					if err == io.EOF || err == io.ErrUnexpectedEOF {
						q.Push(nil)
						break
					} else if err != nil {
						q.Push(nil)
						// 如果删除，那么后面的client可能会重新请求该uri，否则后面的client将永远重定向该uri
						cachedUrl.Lock()
						delete(cachedUrl.m, uri) //delete cachedUrl.a hard
						cachedUrl.Unlock()

						break
					}
				}
			}() // goroutine for receving datas
		} else {
			cachedUrl.Unlock()
		}

		glog.V(1).Infof("[%s] getting %s\n", r.RemoteAddr, uri)

		resp := q.PeekResp()
		if resp == nil {
			glog.V(1).Infof("[%s, nil, 302] %s\n", r.RemoteAddr, uri)
			sendRedir(w, r.URL)
			return
		}
		for k, v := range resp.Header {
			for _, vv := range v {
				w.Header().Add(k, vv)
			}
		}

		clientIp := r.RemoteAddr[:strings.IndexByte(r.RemoteAddr, ':')]
		st.ipLock.Lock()
		st.ipMap[clientIp] = 1
		st.ipLock.Unlock()

		var node *queue.Node
		var buf []byte
		for {
			buf, node = q.Peek(node)
			if buf == nil {
				break
			}

			n, _ := w.Write(buf)
			atomic.AddInt64(&st.serviceSize, int64(n))
		}
	} else if category == '1' {
		cachedStream.Lock()
		q, ok := cachedStream.m[uri]
		if !ok {
			q = queue.NewStreamQueue()
			cachedStream.m[uri] = q
			cachedStream.Unlock()

			glog.V(1).Infof("start living %s [%s]\n", uri, r.RemoteAddr)

			go func() {
				defer func() {
					cachedStream.Lock()
					delete(cachedStream.m, uri)
					cachedStream.Unlock()
				}()

				resp, err := http.DefaultClient.Do(r)
				if err != nil || resp.StatusCode != 200 {
					q.PushResp(nil)
					if err != nil {
						glog.V(1).Infof("[%v] %s\n", err, uri)
					} else {
						resp.Body.Close()
						glog.V(1).Infof("[http %d] %s\n", resp.StatusCode, uri)
					}
					return
				}
				defer resp.Body.Close()

				q.PushResp(resp)

				myflv := flv.NewFlv(resp.Body)
				buf, err := myflv.BasicFlvBlock()
				if err != nil {
					q.PushBFB(nil)
					glog.V(1).Infof("[%v, BasicFlvBlock return] %s\n", err, uri)
					return
				}
				q.PushBFB(buf)
				atomic.AddInt64(&st.cacheSize, int64(len(buf)))

				for {
					buf, err = myflv.Read()
					if err != nil {
						q.Push(nil)
						glog.V(1).Infof("[%v, flv read return] %s\n", err, uri)
						break
					}
					q.Push(buf)
					atomic.AddInt64(&st.cacheSize, int64(len(buf)))

					if atomic.LoadInt64(&st.isExit) > 0 {
						glog.V(1).Infof("stop living %s\n", uri)
						break
					}
				}
			}() // goroutine for receving datas
		} else {
			cachedStream.Unlock()
		}

		glog.V(1).Infof("[%s] watching %s\n", r.RemoteAddr, uri)

		resp := q.PeekResp()
		if resp == nil {
			glog.V(1).Infof("[%s, nil, 302] %s\n", r.RemoteAddr, uri)
			sendRedir(w, r.URL)
			return
		}
		for k, v := range resp.Header {
			for _, vv := range v {
				w.Header().Add(k, vv)
			}
		}

		if buf := q.PeekBFB(); buf != nil {
			atomic.AddInt64(&st.online, 1)

			n, _ := w.Write(buf)
			atomic.AddInt64(&st.serviceSize, int64(n))

			for {
				buf = q.Peek()
				if buf == nil {
					break
				}
				n, err = w.Write(buf)
				atomic.AddInt64(&st.serviceSize, int64(n))
				if err != nil {
					glog.V(1).Infof("[%s, %v, leave] %s\n", r.RemoteAddr, err, uri)
					break
				}
			}
			atomic.AddInt64(&st.online, -1)
		}
	}
}

func main() {
	flag.Parse()
	defer glog.Flush()

	runtime.GOMAXPROCS(runtime.NumCPU())
	// rdebug.SetGCPercent(50)
	initLiveId()

	go recvCommand()
	go updateStat()
	go zhiboGC()

	http.HandleFunc("/", zhiboServer)
	http.HandleFunc("/crossdomain.xml", crossdomain)
	server := &http.Server{Addr: ":8080"}
	server.SetKeepAlivesEnabled(true)
	glog.Fatal(server.ListenAndServe())
}
