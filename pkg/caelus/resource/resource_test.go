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

package resource

import (
	"testing"
	"time"

	"github.com/tencent/caelus/pkg/caelus/util"
	"github.com/tencent/caelus/pkg/util/times"
)

var silenceTestCases = []struct {
	describe                    string
	scheduleSilence             bool
	silenceMode                 bool
	secondsDayStart             time.Duration
	secondsDayEnd               time.Duration
	expectScheduleSilence       bool
	expectScheduleDisabledState bool
	expectSilenceMode           bool
}{
	{
		describe:                    "disable schedule",
		scheduleSilence:             false,
		silenceMode:                 false,
		secondsDayStart:             time.Duration(1 * time.Minute),
		secondsDayEnd:               time.Duration(3 * time.Minute),
		expectScheduleSilence:       true,
		expectScheduleDisabledState: true,
		expectSilenceMode:           false,
	},
	{
		describe:                    "entry silence mode",
		scheduleSilence:             false,
		silenceMode:                 false,
		secondsDayStart:             time.Duration(-5 * time.Minute),
		secondsDayEnd:               time.Duration(2 * time.Minute),
		expectScheduleSilence:       true,
		expectScheduleDisabledState: true,
		expectSilenceMode:           true,
	},
	{
		describe:                    "out of silence mode",
		scheduleSilence:             true,
		silenceMode:                 true,
		secondsDayStart:             time.Duration(10 * time.Minute),
		secondsDayEnd:               time.Duration(15 * time.Minute),
		expectScheduleSilence:       false,
		expectScheduleDisabledState: false,
		expectSilenceMode:           false,
	},
}

// TestCheckSilence test checkSilence function
func TestCheckSilence(t *testing.T) {
	for _, tCase := range silenceTestCases {
		t.Logf("resouce check silence testing: %s", tCase.describe)

		now := time.Now()
		m := &offlineOnK8sManager{
			client:                  newMockClientInterface(),
			aheadOfUnschedulePeriod: time.Duration(2 * time.Minute),
			scheduleSilence:         tCase.scheduleSilence,
			silencePeriods: [][2]times.SecondsInDay{
				{genSecondsInDay(now.Add(tCase.secondsDayStart)), genSecondsInDay(now.Add(tCase.secondsDayEnd))},
			},
		}
		// initialize global silence mode variables
		util.SilenceMode = tCase.silenceMode

		// run check silence function
		m.checkSilence()

		if m.scheduleSilence != tCase.expectScheduleSilence {
			t.Fatalf("check silence case(%s) failed, expect schdule silence %v, got %v",
				tCase.describe, tCase.expectScheduleSilence, m.scheduleSilence)
		}
		if m.client.(*mockClientInterface).ScheduledDisabled != tCase.expectScheduleDisabledState {
			t.Fatalf("check silence case(%s) failed, expect schdule disable state %v, got %v",
				tCase.describe, tCase.expectScheduleDisabledState, m.client.(*mockClientInterface).ScheduledDisabled)
		}
		if util.SilenceMode != tCase.expectSilenceMode {
			t.Fatalf("check silence case(%s) failed, expect silence mode %v, got %v",
				tCase.describe, tCase.expectSilenceMode, util.SilenceMode)
		}
	}
}

// initialize timestamp to secondsInDay struct
func genSecondsInDay(timestamp time.Time) times.SecondsInDay {
	return times.SecondsInDay(timestamp.Second() + timestamp.Minute()*60 + timestamp.Hour()*3600)
}
