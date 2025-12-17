// Copyright 2025 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controller

import (
	"context"
	"io"
	"time"

	"github.com/beego/beego/v2/core/logs"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/alibaba/opensandbox/execd/pkg/jupyter/execute"
	"github.com/alibaba/opensandbox/execd/pkg/runtime"
	"github.com/alibaba/opensandbox/execd/pkg/util/safego"
	"github.com/alibaba/opensandbox/execd/pkg/web/model"
)

var sseHeaders = map[string]string{
	"Content-Type":      "text/event-stream",
	"Cache-Control":     "no-cache",
	"Connection":        "keep-alive",
	"X-Accel-Buffering": "no",
}

func (c *basicController) setupSSEResponse() {
	c.EnableRender = false
	for key, value := range sseHeaders {
		c.Ctx.ResponseWriter.Header().Set(key, value)
	}
}

// setServerEventsHandler adapts runtime callbacks to SSE events.
func (c *CodeInterpretingController) setServerEventsHandler(ctx context.Context) runtime.ExecuteResultHook {
	return runtime.ExecuteResultHook{
		OnExecuteInit: func(session string) {
			payload := model.ServerStreamEvent{
				Type:      model.StreamEventTypeInit,
				Text:      session,
				Timestamp: time.Now().UnixMilli(),
			}.ToJSON()

			c.writeSingleEvent("OnExecuteInit", payload, true)

			safego.Go(func() { c.ping(ctx) })
		},
		OnExecuteResult: func(result map[string]any, count int) {
			var mutated map[string]any
			if len(result) > 0 {
				mutated = make(map[string]any)
				for k, v := range result {
					switch k {
					case "text/plain":
						mutated["text"] = v
					default:
						mutated[k] = v
					}
				}
			}

			if count > 0 {
				payload := model.ServerStreamEvent{
					Type:           model.StreamEventTypeCount,
					ExecutionCount: count,
					Timestamp:      time.Now().UnixMilli(),
				}.ToJSON()
				c.writeSingleEvent("OnExecuteResult", payload, true)
			}
			if len(mutated) > 0 {
				payload := model.ServerStreamEvent{
					Type:      model.StreamEventTypeResult,
					Results:   mutated,
					Timestamp: time.Now().UnixMilli(),
				}.ToJSON()
				c.writeSingleEvent("OnExecuteResult", payload, true)
			}
		},
		OnExecuteComplete: func(executionTime time.Duration) {
			payload := model.ServerStreamEvent{
				Type:          model.StreamEventTypeComplete,
				ExecutionTime: executionTime.Milliseconds(),
				Timestamp:     time.Now().UnixMilli(),
			}.ToJSON()

			c.writeSingleEvent("OnExecuteComplete", payload, true)
		},
		OnExecuteError: func(err *execute.ErrorOutput) {
			if err == nil {
				return
			}

			payload := model.ServerStreamEvent{
				Type:      model.StreamEventTypeError,
				Error:     err,
				Timestamp: time.Now().UnixMilli(),
			}.ToJSON()

			c.writeSingleEvent("OnExecuteError", payload, true)
		},
		OnExecuteStatus: func(status string) {
			payload := model.ServerStreamEvent{
				Type:      model.StreamEventTypeStatus,
				Text:      status,
				Timestamp: time.Now().UnixMilli(),
			}.ToJSON()

			c.writeSingleEvent("OnExecuteStatus", payload, true)
		},
		OnExecuteStdout: func(text string) {
			if text == "" {
				return
			}

			payload := model.ServerStreamEvent{
				Type:      model.StreamEventTypeStdout,
				Text:      text,
				Timestamp: time.Now().UnixMilli(),
			}.ToJSON()

			c.writeSingleEvent("OnExecuteStdout", payload, true)
		},
		OnExecuteStderr: func(text string) {
			if text == "" {
				return
			}

			payload := model.ServerStreamEvent{
				Type:      model.StreamEventTypeStderr,
				Text:      text,
				Timestamp: time.Now().UnixMilli(),
			}.ToJSON()

			c.writeSingleEvent("OnExecuteStderr", payload, true)
		},
	}
}

// writeSingleEvent serializes one SSE frame.
func (c *CodeInterpretingController) writeSingleEvent(handler string, data []byte, verbose bool) {
	if c == nil || c.Ctx == nil || c.Ctx.ResponseWriter == nil {
		return
	}

	select {
	case <-c.Ctx.Request.Context().Done():
		logs.Error("StreamEvent.%s: client disconnected", handler)
		return
	default:
	}

	c.chunkWriter.Lock()
	defer c.chunkWriter.Unlock()
	defer c.Ctx.ResponseWriter.Flush()

	payload := append(data, '\n', '\n')
	n, err := c.Ctx.ResponseWriter.Write(payload)
	if err == nil && n != len(payload) {
		err = io.ErrShortWrite
	}

	if err != nil {
		logs.Error("StreamEvent.%s write data %s error: %v", handler, string(data), err)
	} else {
		if verbose {
			logs.Info("StreamEvent.%s write data %s", handler, string(data))
		}
	}
}

// ping periodically keeps the SSE connection alive.
func (c *CodeInterpretingController) ping(ctx context.Context) {
	wait.Until(func() {
		if c.Ctx.ResponseWriter == nil {
			return
		}
		payload := model.ServerStreamEvent{
			Type:      model.StreamEventTypePing,
			Text:      "pong",
			Timestamp: time.Now().UnixMilli(),
		}.ToJSON()
		c.writeSingleEvent("Ping", payload, false)
	}, 3*time.Second, ctx.Done())
}
