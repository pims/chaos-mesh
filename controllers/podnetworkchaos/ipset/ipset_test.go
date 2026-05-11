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
	"testing"

	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/chaos-mesh/chaos-mesh/api/v1alpha1"
)

func Test_generateIPSetName(t *testing.T) {
	g := NewWithT(t)
	postfix := "alongpostfix"

	t.Run("name with postfix", func(t *testing.T) {
		chaosName := "test"

		networkChaos := &v1alpha1.NetworkChaos{
			ObjectMeta: metav1.ObjectMeta{
				Name: chaosName,
			},
		}

		name := GenerateIPSetName(networkChaos, postfix)

		g.Expect(name).Should(Equal(chaosName + "_" + postfix))
	})

	t.Run("length equal 27", func(t *testing.T) {
		networkChaos := &v1alpha1.NetworkChaos{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-metav1object",
			},
		}

		name := GenerateIPSetName(networkChaos, postfix)

		g.Expect(len(name)).Should(Equal(27))
	})
}

func TestBuildIPSets_IPv6Split(t *testing.T) {
	g := NewWithT(t)

	networkchaos := &v1alpha1.NetworkChaos{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}

	t.Run("ipv4 only returns 2 sets", func(t *testing.T) {
		cidrs := []v1alpha1.CidrAndPort{
			{Cidr: "10.0.0.0/24"},
			{Cidr: "192.168.1.0/24", Port: 80},
		}
		sets := BuildIPSets(nil, cidrs, networkchaos, "postfix", "source")
		g.Expect(len(sets)).Should(Equal(2))
	})

	t.Run("ipv6 only creates additional sets", func(t *testing.T) {
		cidrs := []v1alpha1.CidrAndPort{
			{Cidr: "2001:db8::/32"},
			{Cidr: "::1/128", Port: 53},
		}
		sets := BuildIPSets(nil, cidrs, networkchaos, "postfix", "source")
		// 2 base (empty v4) + 1 net6 + 1 np6
		g.Expect(len(sets)).Should(Equal(4))
	})

	t.Run("mixed v4 and v6 splits correctly", func(t *testing.T) {
		cidrs := []v1alpha1.CidrAndPort{
			{Cidr: "10.0.0.0/24"},
			{Cidr: "2001:db8::/32"},
			{Cidr: "192.168.1.1/32", Port: 80},
			{Cidr: "::1/128", Port: 53},
		}
		sets := BuildIPSets(nil, cidrs, networkchaos, "postfix", "source")
		// 2 base v4 + 1 net6 + 1 np6
		g.Expect(len(sets)).Should(Equal(4))

		// First set is v4 net
		g.Expect(sets[0].Cidrs).Should(Equal([]string{"10.0.0.0/24"}))
		// Second set is v4 net,port
		g.Expect(sets[1].CidrAndPorts).Should(Equal([]v1alpha1.CidrAndPort{{Cidr: "192.168.1.1/32", Port: 80}}))
		// Third set is v6 net
		g.Expect(sets[2].Cidrs).Should(Equal([]string{"2001:db8::/32"}))
		// Fourth set is v6 net,port
		g.Expect(sets[3].CidrAndPorts).Should(Equal([]v1alpha1.CidrAndPort{{Cidr: "::1/128", Port: 53}}))
	})

	t.Run("pod IPs are split by family", func(t *testing.T) {
		pods := []v1.Pod{
			{Status: v1.PodStatus{PodIP: "10.244.1.5"}},
			{Status: v1.PodStatus{PodIP: "fd00::5"}},
		}
		sets := BuildIPSets(pods, nil, networkchaos, "postfix", "source")
		// 2 base v4 + 1 net6
		g.Expect(len(sets)).Should(Equal(3))
		g.Expect(sets[0].Cidrs).Should(Equal([]string{"10.244.1.5/32"}))
		g.Expect(sets[2].Cidrs).Should(Equal([]string{"fd00::5/128"}))
	})
}
