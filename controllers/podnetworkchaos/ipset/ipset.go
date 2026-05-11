// Copyright 2021 Chaos Mesh Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package ipset

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/chaos-mesh/chaos-mesh/api/v1alpha1"
	"github.com/chaos-mesh/chaos-mesh/controllers/chaosimpl/utils"
	"github.com/chaos-mesh/chaos-mesh/controllers/podnetworkchaos/netutils"
	chaosdaemonclient "github.com/chaos-mesh/chaos-mesh/pkg/chaosdaemon/client"
	pb "github.com/chaos-mesh/chaos-mesh/pkg/chaosdaemon/pb"
)

var log = ctrl.Log.WithName("ipset")

// isIPv6Cidr returns true if the given CIDR string is an IPv6 CIDR.
func isIPv6Cidr(cidr string) bool {
	ip, _, err := net.ParseCIDR(cidr)
	if err != nil {
		// Fallback: check for colon which indicates IPv6
		return strings.Contains(cidr, ":")
	}
	return ip.To4() == nil
}

// BuildIPSets builds IP sets with provided pod ip list.
// IPv4 and IPv6 CIDRs are split into separate ipsets because Linux ipset
// cannot mix address families in a single hash:net or hash:net,port set.
func BuildIPSets(pods []v1.Pod, externalCidrs []v1alpha1.CidrAndPort, networkchaos *v1alpha1.NetworkChaos, namePostFix string, source string) []v1alpha1.RawIPSet {
	var cidrs4, cidrs6 []string
	var cidrAndPorts4, cidrAndPorts6 []v1alpha1.CidrAndPort

	for _, cidr := range externalCidrs {
		if cidr.Port == 0 {
			if isIPv6Cidr(cidr.Cidr) {
				cidrs6 = append(cidrs6, cidr.Cidr)
			} else {
				cidrs4 = append(cidrs4, cidr.Cidr)
			}
		} else {
			if isIPv6Cidr(cidr.Cidr) {
				cidrAndPorts6 = append(cidrAndPorts6, cidr)
			} else {
				cidrAndPorts4 = append(cidrAndPorts4, cidr)
			}
		}
	}

	for _, pod := range pods {
		if len(pod.Status.PodIP) > 0 {
			cidr := netutils.IPToCidr(pod.Status.PodIP)
			if isIPv6Cidr(cidr) {
				cidrs6 = append(cidrs6, cidr)
			} else {
				cidrs4 = append(cidrs4, cidr)
			}
		}
	}

	sets := []v1alpha1.RawIPSet{
		{
			Name:      GenerateIPSetName(networkchaos, "net_"+namePostFix),
			IPSetType: v1alpha1.NetIPSet,
			Cidrs:     cidrs4,
			RawRuleSource: v1alpha1.RawRuleSource{
				Source: source,
			},
		},
		{
			Name:         GenerateIPSetName(networkchaos, "netport_"+namePostFix),
			IPSetType:    v1alpha1.NetPortIPSet,
			CidrAndPorts: cidrAndPorts4,
			RawRuleSource: v1alpha1.RawRuleSource{
				Source: source,
			},
		},
	}

	if len(cidrs6) > 0 {
		sets = append(sets, v1alpha1.RawIPSet{
			Name:      GenerateIPSetName(networkchaos, "net6_"+namePostFix),
			IPSetType: v1alpha1.NetIPSet,
			Cidrs:     cidrs6,
			RawRuleSource: v1alpha1.RawRuleSource{
				Source: source,
			},
		})
	}

	if len(cidrAndPorts6) > 0 {
		sets = append(sets, v1alpha1.RawIPSet{
			Name:         GenerateIPSetName(networkchaos, "np6_"+namePostFix),
			IPSetType:    v1alpha1.NetPortIPSet,
			CidrAndPorts: cidrAndPorts6,
			RawRuleSource: v1alpha1.RawRuleSource{
				Source: source,
			},
		})
	}

	return sets
}

// BuildSetIPSet builds list:set IP set that stores given sets
func BuildSetIPSet(sets []v1alpha1.RawIPSet, networkchaos *v1alpha1.NetworkChaos, namePostFix string, source string) v1alpha1.RawIPSet {
	name := GenerateIPSetName(networkchaos, "set_"+namePostFix)
	setNames := []string{}

	for _, set := range sets {
		setNames = append(setNames, set.Name)
	}

	return v1alpha1.RawIPSet{
		Name:      name,
		IPSetType: v1alpha1.SetIPSet,
		SetNames:  setNames,
		RawRuleSource: v1alpha1.RawRuleSource{
			Source: source,
		},
	}
}

// GenerateIPSetName generates name for ipset
func GenerateIPSetName(networkchaos *v1alpha1.NetworkChaos, namePostFix string) string {
	return netutils.CompressName(networkchaos.Name, 27, namePostFix)
}

// FlushIPSets makes grpc calls to chaosdaemon to save ipset
func FlushIPSets(ctx context.Context, pbClient chaosdaemonclient.ChaosDaemonClientInterface, pod *v1.Pod, ipsets []*pb.IPSet) error {
	var err error

	if len(pod.Status.ContainerStatuses) == 0 {
		err = errors.Wrapf(utils.ErrContainerNotFound, "pod %s/%s has empty container status", pod.Namespace, pod.Name)
		return err
	}

	log.Info("Flushing IP Sets....")
	for _, containerStatus := range pod.Status.ContainerStatuses {
		containerID := containerStatus.ContainerID
		log.Info("attempting to flush ip set", "containerID", containerID)

		_, err = pbClient.FlushIPSets(ctx, &pb.IPSetsRequest{
			Ipsets:      ipsets,
			ContainerId: containerID,
			EnterNS:     true,
			PodUid:      string(pod.UID),
		})

		if err != nil {
			log.Error(err, fmt.Sprintf("error while flushing ip sets for containerID %s", containerID))
		} else {
			log.Info("Successfully flushed ip set")
			return nil
		}
	}

	return errors.Errorf("unable to flush ip sets for pod %s", pod.Name)
}
