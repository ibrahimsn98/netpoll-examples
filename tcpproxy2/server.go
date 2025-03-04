/*
 * Copyright 2021 CloudWeGo
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/cloudwego/netpoll"
)

var (
	downstreamAddr = "127.0.0.1:8080"
	downstreamKey  = "downstream"
)

func main() {
	network, address := "tcp", ":8081"
	listener, _ := netpoll.CreateListener(network, address)
	eventLoop, _ := netpoll.NewEventLoop(
		onRequest,
		netpoll.WithOnConnect(onConnect),
		netpoll.WithReadTimeout(time.Second),
	)

	// start listen loop ...
	if err := eventLoop.Serve(listener); err != nil {
		log.Fatal(err)
	}
}

var _ netpoll.OnConnect = onConnect
var _ netpoll.OnRequest = onRequest

func onConnect(ctx context.Context, _ netpoll.Connection) context.Context {
	downstream, err := netpoll.DialConnection("tcp", downstreamAddr, time.Second)
	if err != nil {
		log.Printf("connect downstream failed: %v", err)
	}
	return context.WithValue(ctx, downstreamKey, downstream)
}

func onRequest(ctx context.Context, upstream netpoll.Connection) error {
	downstream := ctx.Value(downstreamKey).(netpoll.Connection)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		transfer(ctx, upstream, downstream)
	}()

	go func() {
		defer wg.Done()
		transfer(ctx, downstream, upstream)
	}()

	wg.Wait()
	if err := upstream.Close(); err != nil {
		fmt.Printf("close downstream connection failed: %v", err)
	}
	if err := downstream.Close(); err != nil {
		fmt.Printf("close downstream connection failed: %v", err)
	}
	return nil
}

func transfer(ctx context.Context, src netpoll.Connection, dst netpoll.Connection) {
	reader := src.Reader()
	writer := dst.Writer()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			buf, err := reader.ReadBinary(reader.Len())
			if err != nil {
				fmt.Printf("read stream failed: %v", err)
				return
			}

			alloc, err := writer.Malloc(len(buf))
			if err != nil {
				fmt.Printf("malloc writer failed: %v", err)
				return
			}

			copy(alloc, buf)

			if writer.MallocLen() > 0 {
				if err = writer.Flush(); err != nil {
					fmt.Printf("flush writer error: %v", err)
					return
				}
			}
			if err = reader.Release(); err != nil {
				fmt.Printf("release reader error: %v", err)
				return
			}
		}
	}
}
