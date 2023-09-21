/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package options

import (
	"fmt"
	"net"
	"strings"

	apiserveroptions "k8s.io/apiserver/pkg/server/options"
	_ "k8s.io/component-base/metrics/prometheus/workqueue"
	netutils "k8s.io/utils/net"

	controlplane "k8s.io/kubernetes/pkg/controlplane/apiserver/options"
	"k8s.io/kubernetes/pkg/kubeapiserver"
	kubeoptions "k8s.io/kubernetes/pkg/kubeapiserver/options"
)

// completedOptions is a private wrapper that enforces a call of Complete() before Run can be invoked.
type completedOptions struct {
	controlplane.CompletedOptions
	CloudProvider *kubeoptions.CloudProviderOptions

	Extra
}

type CompletedOptions struct {
	// Embed a private pointer that cannot be instantiated outside of this package.
	*completedOptions
}

// Complete set default ServerRunOptions.
// Should be called after kube-apiserver flags parsed.
func (opts *ServerRunOptions) Complete() (CompletedOptions, error) {
	if opts == nil {
		return CompletedOptions{completedOptions: &completedOptions{}}, nil
	}

	// process opts.ServiceClusterIPRange from list to Primary and Secondary
	// we process secondary only if provided by user
	// 获取apiserver ip 和 主要的 service ip 范围
	apiServerServiceIP, primaryServiceIPRange, secondaryServiceIPRange, err := getServiceIPAndRanges(opts.ServiceClusterIPRanges)
	if err != nil {
		return CompletedOptions{}, err
	}
	controlplane, err := opts.Options.Complete([]string{"kubernetes.default.svc", "kubernetes.default", "kubernetes"}, []net.IP{apiServerServiceIP})
	if err != nil {
		return CompletedOptions{}, err
	}

	completed := completedOptions{
		CompletedOptions: controlplane,
		CloudProvider:    opts.CloudProvider,

		Extra: opts.Extra,
	}

	completed.PrimaryServiceClusterIPRange = primaryServiceIPRange
	completed.SecondaryServiceClusterIPRange = secondaryServiceIPRange
	completed.APIServerServiceIP = apiServerServiceIP

	if completed.Etcd != nil && completed.Etcd.EnableWatchCache {
		sizes := kubeapiserver.DefaultWatchCacheSizes()
		// Ensure that overrides parse correctly.
		userSpecified, err := apiserveroptions.ParseWatchCacheSizes(completed.Etcd.WatchCacheSizes)
		if err != nil {
			return CompletedOptions{}, err
		}
		for resource, size := range userSpecified {
			sizes[resource] = size
		}
		completed.Etcd.WatchCacheSizes, err = apiserveroptions.WriteWatchCacheSizes(sizes)
		if err != nil {
			return CompletedOptions{}, err
		}
	}

	return CompletedOptions{
		completedOptions: &completed,
	}, nil
}

// CIDR 表示法（Classless Inter-Domain Routing notation）是一种用于表示IP地址块和子网掩码的标准记法。
// IDR表示法使用斜线后跟一个数字来表示子网掩码位数，例如，`192.168.0.0/24`。在这个例子中，`192.168.0.0`是网络前缀，`/24`表示子网掩码为24位，即前24位是网络部分，后8位是主机部分。
// 1. 将`192.168.0.0`转换为32位的二进制表示：`11000000.10101000.00000000.00000000`。
// 2. 根据子网掩码的位数，确定网络部分和主机部分的范围。在这种情况下，前24位是网络部分，后8位是主机部分。
// 3. 确定网络部分的最小和最大值。对于前24位固定的网络部分，最小值是`192.168.0.0`，最大值是`192.168.0.255`。
// 4. 因此，`192.168.0.0/24`表示的IP范围是从`192.168.0.0`到`192.168.0.255`。

// 参数：serviceClusterIPRanges， 格式：192.168.0.0/24，10.16.0.0/16
func getServiceIPAndRanges(serviceClusterIPRanges string) (net.IP, net.IPNet, net.IPNet, error) {
	serviceClusterIPRangeList := []string{}
	if serviceClusterIPRanges != "" {
		serviceClusterIPRangeList = strings.Split(serviceClusterIPRanges, ",")
	}
	// api server ip
	var apiServerServiceIP net.IP
	// 主要的service ip 范围
	var primaryServiceIPRange net.IPNet
	// 次要的service ip 范围
	var secondaryServiceIPRange net.IPNet
	var err error
	// nothing provided by user, use default range (only applies to the Primary)
	// 命令行没有提供集群ip地址范围， 使用默认的范围， 仅仅是提供主要的service ip范围
	if len(serviceClusterIPRangeList) == 0 {
		var primaryServiceClusterCIDR net.IPNet
		primaryServiceIPRange, apiServerServiceIP, err = controlplane.ServiceIPRange(primaryServiceClusterCIDR)
		if err != nil {
			return net.IP{}, net.IPNet{}, net.IPNet{}, fmt.Errorf("error determining service IP ranges: %v", err)
		}
		return apiServerServiceIP, primaryServiceIPRange, net.IPNet{}, nil
	}
	// 得到主要service 集群的cidr
	_, primaryServiceClusterCIDR, err := netutils.ParseCIDRSloppy(serviceClusterIPRangeList[0])
	if err != nil {
		return net.IP{}, net.IPNet{}, net.IPNet{}, fmt.Errorf("service-cluster-ip-range[0] is not a valid cidr")
	}
	// 得到主要的service range， api server ip
	primaryServiceIPRange, apiServerServiceIP, err = controlplane.ServiceIPRange(*primaryServiceClusterCIDR)
	if err != nil {
		return net.IP{}, net.IPNet{}, net.IPNet{}, fmt.Errorf("error determining service IP ranges for primary service cidr: %v", err)
	}

	// user provided at least two entries
	// note: validation asserts that the list is max of two dual stack entries
	// ip range list len > 1 表示用户提供了两个地址段， 这里解析次要地址段
	if len(serviceClusterIPRangeList) > 1 {
		_, secondaryServiceClusterCIDR, err := netutils.ParseCIDRSloppy(serviceClusterIPRangeList[1])
		if err != nil {
			return net.IP{}, net.IPNet{}, net.IPNet{}, fmt.Errorf("service-cluster-ip-range[1] is not an ip net")
		}
		secondaryServiceIPRange = *secondaryServiceClusterCIDR
	}
	return apiServerServiceIP, primaryServiceIPRange, secondaryServiceIPRange, nil
}
