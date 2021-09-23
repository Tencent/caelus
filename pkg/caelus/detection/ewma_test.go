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

package detection

import (
	"math"
	"testing"
	"time"
)

var epsilon = 0.00000001

func TestMean(t *testing.T) {
	cases := []struct {
		name string
		data []float64
		mean float64
	}{
		{
			name: "test_mean1",
			data: []float64{7787, 7585, 7970, 7117, 7870, 8611, 8112, 8490, 8787, 7592,
				8392, 8104, 8412, 8394, 8802, 8621},
			mean: 8507.645267489714,
		},
		{
			name: "test_mean2",
			data: []float64{8392, 8104, 8412, 8394, 8802, 8621},
			mean: 0, // not enough data, EWMA is not ready for producing data.
		},
	}
	for _, test := range cases {
		d := initDetector(test.data, 5)
		m := d.Mean()
		if !isEqualFloat64(m, test.mean) {
			t.Errorf("%s faild, expected %v, got %v", test.name, test.mean, m)
		}
	}
}

func TestStdDev(t *testing.T) {
	cases := []struct {
		name   string
		data   []float64
		stddev float64
	}{
		{
			name: "test_stddev1",
			data: []float64{7787, 7585, 7970, 7117, 7870, 8611, 8112, 8490, 8787, 7592,
				8392, 8104, 8412, 8394, 8802, 8621},
			stddev: 238.53166243352715,
		},
	}
	for _, test := range cases {
		d := initDetector(test.data, 5)
		m := d.StdDev()
		if !isEqualFloat64(m, test.stddev) {
			t.Errorf("%s faild, expected %v, got %v", test.name, test.stddev, m)
		}
	}
}

func TestISAnomaly(t *testing.T) {
	// Data is from https://github.com/sjtu-cs222/Group_43/blob/master/A1Benchmark/real_11.csv
	var data []float64 = []float64{7787, 7585, 7970, 7117, 7870, 8611, 8112, 8490, 8787, 7592,
		8392, 8104, 8412, 8394, 8802, 8621, 9456, 9456, 10868, 10789, 11837, 12189}
	cases := []struct {
		newData   float64
		isAnomaly bool
	}{
		{18112, true},
		{8461, false},
	}
	for _, test := range cases {
		d := initDetector(data, 20)
		d.Add(TimedData{Ts: time.Now(), Vals: map[string]float64{"data": test.newData}})
		got, err := d.IsAnomaly()
		if err != nil {
			t.Errorf("Failed to detect new data %v, got err: %v", test.newData, err)
		} else {
			if got != test.isAnomaly {
				t.Errorf("Failed to dectect new data %v, expected %v, got %v", test.newData, test.isAnomaly, got)
			}
		}
	}
}

func initDetector(data []float64, n int) *EwmaDetector {
	detector := NewEwmaDetector("data", n)
	count := len(data) + 1
	for i, d := range data {
		detector.Add(TimedData{Ts: time.Now().Add(-time.Duration(count - i)), Vals: map[string]float64{"data": d}})
	}
	return detector
}

func isEqualFloat64(a, b float64) bool {
	// The following algorithm is not right, but it is OK for test now.
	// For more information, refer https://floating-point-gui.de/errors/comparison/#look-out-for-edge-cases.
	return math.Abs(a-b) < epsilon
}
