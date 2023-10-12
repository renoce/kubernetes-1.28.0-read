## k8s中一些疑问：

1. aggregator -  master - extension  delegate这个请求链路上， 除了在aggregator上发生一次登录和鉴权， 在master和extension上会不会再次发生？

答案：不会， 这显然是多余的验证， 在形成delegation链（server chain）时， 下一个server的unprotectedHandler被使用， 他是未经过filter包裹的， 自然不会发生登录和鉴权等.


vendor/k8s.io/apiserver/pkg/server/genericapiserver.go中

func (s preparedGenericAPIServer) NonBlockingRun函数中stoppedCh, listenerStoppedCh, err = s.SecureServingInfo.Serve(s.Handler, shutdownTimeout, internalStopCh)

启动serve传入的s.Handler是Aggregator中的GenericServer中的Handler， 这个handler是

vendor/k8s.io/apiserver/pkg/server/config.go 中
apiServerHandler := NewAPIServerHandler(name, c.Serializer, handlerChainBuilder, delegationTarget.UnprotectedHandler()) 这个handler，

这是一个完整的Handler， 当

// ServeHTTP makes it an http.Handler
func (a *APIServerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 先遍历handler chain， 最后调用director的ServeHttp
	a.FullHandlerChain.ServeHTTP(w, r)
}

遍历完handler chain后调用director ServeHTTP，

```
func (d director) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := req.URL.Path

	// check to see if our webservices want to claim this path
	for _, ws := range d.goRestfulContainer.RegisteredWebServices() {
		switch {
		case ws.RootPath() == "/apis":
			// if we are exactly /apis or /apis/, then we need special handling in loop.
			// normally these are passed to the nonGoRestfulMux, but if discovery is enabled, it will go directly.
			// We can't rely on a prefix match since /apis matches everything (see the big comment on Director above)
			if path == "/apis" || path == "/apis/" {
				klog.V(5).Infof("%v: %v %q satisfied by gorestful with webservice %v", d.name, req.Method, path, ws.RootPath())
				// don't use servemux here because gorestful servemuxes get messed up when removing webservices
				// TODO fix gorestful, remove TPRs, or stop using gorestful
				d.goRestfulContainer.Dispatch(w, req)
				return
			}

		case strings.HasPrefix(path, ws.RootPath()):
			// ensure an exact match or a path boundary match
			if len(path) == len(ws.RootPath()) || path[len(ws.RootPath())] == '/' {
				klog.V(5).Infof("%v: %v %q satisfied by gorestful with webservice %v", d.name, req.Method, path, ws.RootPath())
				// don't use servemux here because gorestful servemuxes get messed up when removing webservices
				// TODO fix gorestful, remove TPRs, or stop using gorestful
				d.goRestfulContainer.Dispatch(w, req)
				return
			}
		}
	}

	// if we didn't find a match, then we just skip gorestful altogether
	klog.V(5).Infof("%v: %v %q satisfied by nonGoRestful", d.name, req.Method, path)
    // 如果找不到service， 会到达上一层server    aggregator server -> master server -> extension server -> nohandler
	d.nonGoRestfulMux.ServeHTTP(w, req)
}
```


如果没有找到对应aggregator server下的 service handler， 会调用 到达 Master server， 调用d.nonGoRestfulMux.ServeHTTP， 这里的nonGoRestfulMux其实是构建aggregator server的时候传入的server的时候传入的UnprotectedHandler， 由GenericServer实现， 

实现
```
func (s *GenericAPIServer) UnprotectedHandler() http.Handler {
	// when we delegate, we need the server we're delegating to choose whether or not to use gorestful
	return s.Handler.Director
}
```

```
apiServerHandler := NewAPIServerHandler(name, c.Serializer, handlerChainBuilder, delegationTarget.UnprotectedHandler())
```

在调用Master server的时候， 调用的delegationTarget.UnprotectedHandler, 其实调用的由 aggregator 调用时的s.Handler 变成了调用了s.Handler.Director， 略过了 APIServerHandler


```
func (a *APIServerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 先遍历handler chain， 最后调用director的ServeHttp
	a.FullHandlerChain.ServeHTTP(w, r)
}
```

不会遍历所有的handler chain， 直接调用了最终的 director handler。

master server 如果找不到对应的接口 服务， 当接口传给 extension server 也是一样的， 不会调用 handler chain， 直到 no handler server 结束整个 server chain.








2. RunPostStartHooks启动后置钩子函数， master， extension， aggregator的钩子函数都启动了吗， 在3个server中， 钩子函数都属于genericserver， 他们的使用顺序？



3. aggregator， master， extension server3种 server的Handler如何调用具体的各个server的api的处理函数的， 各个handler和各个server的api处理函数怎么关联的？