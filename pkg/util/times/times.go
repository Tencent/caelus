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

package times

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Duration is a alias of time.Duration which supports correct
// marshaling to YAML and JSON
type Duration time.Duration

// MarshalJSON marshals Duration
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// UnmarshalJSON unmarshals Duration
func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case float64:
		*d = Duration(time.Duration(value))
		return nil
	case string:
		if len(value) == 0 {
			*d = Duration(time.Duration(0))
			return nil
		}

		tmp, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		*d = Duration(tmp)
		return nil
	default:
		return errors.New("invalid duration")
	}
}

// Seconds return the duration as a floating point number of seconds.
func (d Duration) Seconds() float64 {
	return time.Duration(d).Seconds()
}

// TimeDuration convert duration to time.Duration
func (d Duration) TimeDuration() time.Duration {
	return time.Duration(d)
}

// SecondsInDay show timestamp in second format in one day
type SecondsInDay uint32

// MarshalJSON translate secondsInDay struct data to string
func (s SecondsInDay) MarshalJSON() ([]byte, error) {
	seconds := s
	return []byte(fmt.Sprintf(`"%02d:%02d:%02d"`, seconds/3600, seconds%3600/60, seconds%3600%60)), nil
}

// UnmarshalJSON translate string to secondsInDay struct
func (s *SecondsInDay) UnmarshalJSON(d []byte) error {
	str := strings.Trim(string(d), "\"")
	parts := strings.Split(str, ":")
	if len(parts) <= 1 || len(parts) > 3 {
		return fmt.Errorf("expect format 02:12 or 02:12:00, got %s", str)
	}
	var seconds uint32
	for i := 0; i <= 2; i++ {
		if i < len(parts) {
			t, err := strconv.ParseUint(parts[i], 10, 32)
			if err != nil {
				return fmt.Errorf("expect format 02:12 or 02:12:00, got %s", str)
			}
			seconds = seconds*60 + uint32(t)
		} else {
			seconds = seconds * 60
		}
	}
	*s = SecondsInDay(seconds)
	return nil
}

// String output secondsInDay in string format
func (s SecondsInDay) String() string {
	str, _ := s.MarshalJSON()
	return string(str)
}

// IsTimeInSecondsDay check if the timestamp is in range of the two secondsInDay timestamp
func IsTimeInSecondsDay(timestamp time.Time, ss [2]SecondsInDay) bool {
	seconds := SecondsInDay(timestamp.Second() + timestamp.Minute()*60 + timestamp.Hour()*3600)
	if seconds >= ss[0] && seconds <= ss[1] {
		return true
	}
	return false
}
