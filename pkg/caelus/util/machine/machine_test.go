/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 *
 * You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package machine

import (
	"strings"
	"testing"
)

type memoryLimitTestData struct {
	describe string
	limit    int64
	usage    int64
	expect   struct {
		limit  int64
		reason string
	}
}

// TestGetMemoryCgroupLimitByUsage test memory cgroup limit setting
func TestGetMemoryCgroupLimitByUsage(t *testing.T) {
	testCases := []memoryLimitTestData{
		{
			describe: "small usage",
			limit:    10 * 1024 * 1024 * 1024,
			usage:    10 * 1024 * 1024,
			expect: struct {
				limit  int64
				reason string
			}{
				limit:  10 * 1024 * 1024 * 1024,
				reason: "",
			},
		},
		{
			describe: "usage bigger than limit",
			limit:    10 * 1024 * 1024 * 1024,
			usage:    11 * 1024 * 1024 * 1024,
			expect: struct {
				limit  int64
				reason string
			}{
				limit:  12880707584,
				reason: "less than current usage",
			},
		},
		{
			describe: "usage nearly reach to limit",
			limit:    10.5 * 1024 * 1024 * 1024,
			usage:    10 * 1024 * 1024 * 1024,
			expect: struct {
				limit  int64
				reason string
			}{
				limit:  11806965760,
				reason: "nearly to usage",
			},
		},
		{
			describe: "limit bigger than usage",
			limit:    12 * 1024 * 1024 * 1024,
			usage:    10 * 1024 * 1024,
			expect: struct {
				limit  int64
				reason string
			}{
				limit:  12 * 1024 * 1024 * 1024,
				reason: "",
			},
		},
	}

	for _, tc := range testCases {
		limit, reason := GetMemoryCgroupLimitByUsage(tc.limit, tc.usage)
		if tc.expect.limit != limit {
			t.Fatalf("memory cgroup get limit case(%s) failed, expect %d, got %d",
				tc.describe, tc.expect.limit, limit)
		}
		if reason != tc.expect.reason && !strings.Contains(reason, tc.expect.reason) {
			t.Fatalf("memory cgroup get limit case(%s) failed, expect reason: %s, got: %s",
				tc.describe, tc.expect.reason, reason)
		}
	}
}
