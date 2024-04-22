package main

import (
	"flag"
	"fmt"
	"github.com/gorilla/websocket"
	"io"
	"nesty.cn/bee"
	"nesty.cn/libmyquit"
	"net/http"
	"strconv"
	"time"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// var readPacketMin int = 0
// var readPacketMax int = 0
// var readPacketAvg int = 0
// var readPacketLoops int = 0

func ReadWebSocket(c *websocket.Conn, buff []byte) (int, int, []byte, error) {

	buff = buff[:0]

	var r io.Reader
	messageType, r, err := c.NextReader()
	if err != nil {
		return messageType, 0, buff, err
	}
	nreadloop := 1

	n, err := r.Read(buff)

	if err == io.EOF {
		err = nil
		//fmt.Println("0 totread:",n,"readloop:",nreadloop)
		return messageType, n, buff, err
	}

	nmin := 1024
	nmax := 0
	// n0 := n

	ntot := n
	b := make([]byte, 10240)
	// istart := n
	for {
		nreadloop++
		n, err = r.Read(b)
		if n > 0 {
			//istart, buff = Memcpy(buff, istart, b, n)
			buff = append(buff, b[:n]...)
			ntot += n

			if n < nmin {
				nmin = n
			} else if n > nmax {
				nmax = n
			}
		}

		if err == io.EOF {
			err = nil
			// fmt.Println("nreadloop:", nreadloop, "min:", nmin, "max:", nmax, (ntot-n0)/(nreadloop-1))
			// if ntot > 1000 {
			// 	readPacketMin = nmin
			// 	readPacketMax = nmax
			// 	readPacketLoops = nreadloop
			// 	readPacketAvg = (ntot - n0) / (nreadloop - 1)
			// }
			return messageType, ntot, buff, err
		} else if err != nil {
			break
		}
		// time.Sleep(100 * time.Nanosecond)
	}
	//fmt.Println("2 totread:",ntot,"readloop:",nreadloop)
	return messageType, ntot, buff, err
}

func requestHandler(w http.ResponseWriter, r *http.Request) {

	if bBigReadbuffer {
		upgrader.ReadBufferSize = 1024 * 1024 * 2
	}
	upgrader.EnableCompression = bCompress

	conn, err := upgrader.Upgrade(w, r, nil)
	fmt.Println("header: ", r.Header)

	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("bMemory:", bMemory)

	ntot := int64(0)
	t0 := int64(0)
	speedRecv := 0.0

	go func() {

		readBuf := make([]byte, 0, 1024*1024*10+1)

		for {
			var nread int
			var err error

			readBuf = readBuf[:0]
			if !bMemory {
				_, readBuf, err = conn.ReadMessage()
			} else {
				_, nread, readBuf, err = ReadWebSocket(conn, readBuf) //new code
			}
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					fmt.Println("client closed gracely")
					break
				}
				fmt.Println("Error reading message from client:", err)
				break
			}
			if !bMemory {
				nread = len(readBuf)
			}

			if nread < 1 {
				continue
			}
			p := readBuf

			if p[0] == 1 {
				t0 = time.Now().UnixNano() / 1000000
				ntot = 0
			} else if p[0] == 0 {
				ntot += int64(nread)
			} else if p[0] == 2 {
				t1 := time.Now().UnixNano() / 1000000
				speedRecv = float64(ntot / 1024.0 / 1024 * 1000 / (t1 - t0))
				fmt.Printf("recv speed: %v MB/S, %v MB recv, total time: %v ms\n", speedRecv, (ntot / 1024.0 / 1024), t1-t0)
				bd := []byte{2}
				bs := []byte(fmt.Sprintf("speed %v MB/S", speedRecv))
				bd = append(bd, bs...)
				conn.WriteMessage(websocket.BinaryMessage, bd)
			} else if p[0] == 3 { //ping response
				bd := []byte{3}
				conn.WriteMessage(websocket.BinaryMessage, bd)
			} else if p[0] == 4 {
				t0 = time.Now().UnixNano() / 1000000
				ntot = 0
				tsize := int(p[1])*256 + int(p[2])
				bd := make([]byte, 1024*1024*10)
				bd[0] = 0
				for i := 0; i < tsize; i++ {
					conn.WriteMessage(websocket.BinaryMessage, bd)
					ntot += int64(len(bd))
				}
				t1 := time.Now().UnixNano() / 1000000
				speed := ntot / 1024.0 / 1024 * 1000 / (t1 - t0)
				fmt.Printf("send speed: %v MB/s, %v MB send, total time: %v ms\n", speed, (ntot / 1024.0 / 1024), t1-t0)
			}
		}
		fmt.Println("done ", r)
		conn.Close()
	}()
}

var bMemory = true
var bCompress = false
var bBigReadbuffer = true

func main() {
	fmt.Println("------------main start----------")

	libmyquit.Install()

	var _server *http.Server

	var port int
	var maxfps int
	var smemory string
	var scompress string
	var sbigreadbuffer string
	flag.IntVar(&port, "p", 8080, "port")
	flag.StringVar(&smemory, "m", "true", "enable Bee WebSocket Read")
	flag.StringVar(&scompress, "c", "false", "enable Compress")
	flag.StringVar(&sbigreadbuffer, "b", "true", "enable big read buffer")
	flag.Parse()

	bMemory = bee.Str2Bool(smemory)
	bCompress = bee.Str2Bool(scompress)
	bBigReadbuffer = bee.Str2Bool(sbigreadbuffer)

	bee.Log("enable Bee ReadMessage:", bMemory)
	bee.Log("enable compress:", bCompress)
	bee.Log("enable big read buffer:", bBigReadbuffer)

	if port == 0 && len(flag.Args()) > 0 {
		port, _ = strconv.Atoi(flag.Args()[0])

	}

	if port == 0 {
		port = 8080
	}

	if maxfps < 10 {
		maxfps = 10
	}

	fmt.Println("port:", port)

	http.HandleFunc("/ws", requestHandler)

	_server = &http.Server{Addr: ":" + strconv.Itoa(port), Handler: nil}
	go func() {
		fmt.Println("server running")
		err := _server.ListenAndServe()
		fmt.Println("server done")
		if !libmyquit.IsQuiting() && err != nil {
			libmyquit.TellAllDone()
			fmt.Println("Error starting server:", err)
		}
	}()

	libmyquit.Barrier(func() {
		_server.Close()
	})
}
