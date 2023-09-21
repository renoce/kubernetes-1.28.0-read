## k8s中一些疑问：

1. aggregator -  master - extension  delegate这个请求链路上， 除了在aggregator上发生一次登录和鉴权， 在master何extension上会不会再次发生？

答案：不会， 这显然是多余的验证， 在形成delegation链（server chain）时， 下一个server的unprotectedHandler被使用， 他是未经过filter包裹的， 自然不会发生登录和鉴权等.