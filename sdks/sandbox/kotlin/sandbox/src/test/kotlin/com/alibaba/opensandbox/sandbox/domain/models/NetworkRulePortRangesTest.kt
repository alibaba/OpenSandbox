/*
 * Copyright 2026 The OpenSandbox Authors
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

package com.alibaba.opensandbox.sandbox.domain.models

import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.NetworkPolicy
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.NetworkRule
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.PortRange
import com.alibaba.opensandbox.sandbox.infrastructure.adapters.converter.SandboxModelConverter.toApiEgressNetworkRule
import com.alibaba.opensandbox.sandbox.infrastructure.adapters.converter.SandboxModelConverter.toApiNetworkPolicy
import com.alibaba.opensandbox.sandbox.infrastructure.adapters.converter.SandboxModelConverter.toApiNetworkRule
import com.alibaba.opensandbox.sandbox.infrastructure.adapters.converter.SandboxModelConverter.toDomainEgressNetworkRule
import com.alibaba.opensandbox.sandbox.infrastructure.adapters.converter.SandboxModelConverter.toDomainNetworkRule
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import com.alibaba.opensandbox.sandbox.api.models.NetworkRule as ApiNetworkRule
import com.alibaba.opensandbox.sandbox.api.models.PortRange as ApiPortRange
import com.alibaba.opensandbox.sandbox.api.models.egress.NetworkRule as ApiEgressNetworkRule
import com.alibaba.opensandbox.sandbox.api.models.egress.PortRange as ApiEgressPortRange

class NetworkRulePortRangesTest {
    private val complementOf8000 = listOf(PortRange(1, 7999), PortRange(8001, 65535))

    private fun denyComplement(): NetworkRule =
        NetworkRule
            .builder()
            .action(NetworkRule.Action.DENY)
            .target("10.0.0.5")
            .portRanges(complementOf8000)
            .build()

    @Test
    fun `builder accepts a complement deny for a carved-out host`() {
        val rule = denyComplement()

        assertEquals("10.0.0.5", rule.target)
        assertNull(rule.ports)
        assertEquals(complementOf8000, rule.portRanges)
    }

    @Test
    fun `builder accepts port ranges with no target and alongside disjoint ports`() {
        val rangeOnly =
            NetworkRule
                .builder()
                .action(NetworkRule.Action.DENY)
                .portRanges(listOf(PortRange(6379, 6379)))
                .build()
        assertNull(rangeOnly.target)

        val mixed =
            NetworkRule
                .builder()
                .action(NetworkRule.Action.DENY)
                .target("10.0.0.0/8")
                .ports(listOf(22))
                .portRanges(listOf(PortRange(10001, 10100)))
                .build()
        assertEquals(listOf(22), mixed.ports)
        assertEquals(listOf(PortRange(10001, 10100)), mixed.portRanges)
    }

    @Test
    fun `builder rejects invalid and overlapping port ranges`() {
        for (ranges in listOf(
            emptyList(),
            listOf(PortRange(0, 10)),
            listOf(PortRange(10, 65536)),
            listOf(PortRange(20, 10)),
            listOf(PortRange(1, 100), PortRange(50, 200)),
            listOf(PortRange(100, 200), PortRange(1, 100)),
        )) {
            assertThrows(IllegalArgumentException::class.java, { NetworkRule.builder().portRanges(ranges) }, "$ranges")
        }
    }

    @Test
    fun `build rejects ports inside a range, domain targets, and the combined cap`() {
        assertThrows(IllegalArgumentException::class.java) {
            NetworkRule
                .builder()
                .action(NetworkRule.Action.DENY)
                .ports(listOf(150))
                .portRanges(listOf(PortRange(100, 200)))
                .build()
        }
        assertThrows(IllegalArgumentException::class.java) {
            NetworkRule
                .builder()
                .action(NetworkRule.Action.DENY)
                .target("example.org")
                .portRanges(listOf(PortRange(1, 2)))
                .build()
        }
        assertThrows(IllegalArgumentException::class.java) {
            NetworkRule
                .builder()
                .action(NetworkRule.Action.DENY)
                .ports((1..200).toList())
                .portRanges((0 until 57).map { PortRange(1000 + 2 * it, 1000 + 2 * it) })
                .build()
        }
    }

    @Test
    fun `built rule port ranges are a defensive copy`() {
        val ranges = mutableListOf(PortRange(1, 10))
        val rule =
            NetworkRule
                .builder()
                .action(NetworkRule.Action.DENY)
                .portRanges(ranges)
                .build()

        ranges.add(PortRange(5, 5))
        assertEquals(listOf(PortRange(1, 10)), rule.portRanges)
    }

    @Test
    fun `lifecycle and egress wire shapes carry portRanges and omit them when unset`() {
        val lifecycleJson = Json.encodeToJsonElement(ApiNetworkRule.serializer(), denyComplement().toApiNetworkRule()).jsonObject
        assertEquals(
            """[{"start":1,"end":7999},{"start":8001,"end":65535}]""",
            lifecycleJson.getValue("portRanges").jsonArray.toString(),
        )
        val egressJson =
            Json.encodeToJsonElement(ApiEgressNetworkRule.serializer(), denyComplement().toApiEgressNetworkRule()).jsonObject
        assertEquals(lifecycleJson.getValue("portRanges"), egressJson.getValue("portRanges"))

        val plain = NetworkRule.builder().action(NetworkRule.Action.ALLOW).target("pypi.org").build()
        assertFalse(Json.encodeToJsonElement(ApiNetworkRule.serializer(), plain.toApiNetworkRule()).jsonObject.containsKey("portRanges"))

        val policy = NetworkPolicy.builder().defaultAction(NetworkPolicy.DefaultAction.DENY).egress(listOf(denyComplement())).build()
        assertEquals(
            listOf(ApiPortRange(1, 7999), ApiPortRange(8001, 65535)),
            policy.toApiNetworkPolicy().egress!!.single().portRanges,
        )
    }

    @Test
    fun `read-back maps port ranges and treats an empty list as absent`() {
        val lifecycle =
            ApiNetworkRule(
                action = ApiNetworkRule.Action.deny,
                target = "10.0.0.5",
                portRanges = listOf(ApiPortRange(1, 7999), ApiPortRange(8001, 65535)),
            ).toDomainNetworkRule()
        assertEquals(complementOf8000, lifecycle.portRanges)

        val egress =
            ApiEgressNetworkRule(
                action = ApiEgressNetworkRule.Action.deny,
                target = "",
                portRanges = listOf(ApiEgressPortRange(6379, 6379)),
            ).toDomainEgressNetworkRule()
        assertNull(egress.target)
        assertEquals(listOf(PortRange(6379, 6379)), egress.portRanges)

        val empty =
            ApiNetworkRule(action = ApiNetworkRule.Action.allow, target = "10.0.0.5", portRanges = emptyList())
                .toDomainNetworkRule()
        assertNull(empty.portRanges)
    }

    @Test
    fun `read-back is lenient on out-of-range port ranges`() {
        val rule =
            ApiNetworkRule(
                action = ApiNetworkRule.Action.deny,
                target = "10.0.0.5",
                portRanges = listOf(ApiPortRange(70000, 1)),
            ).toDomainNetworkRule()
        assertTrue(rule.portRanges!!.single() == PortRange(70000, 1))
    }
}
