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
	"testing"
	"time"
)

// A show test struct
type A struct {
	T Duration
}

// TestUnMarshal test unmarshal
func TestUnMarshal(t *testing.T) {
	var a A
	if err := json.Unmarshal([]byte(`{"T":"5m"}`), &a); err != nil {
		t.Fatal(err)
	}
	if time.Duration(a.T).Seconds() != 300 {
		t.Fatal(a.T)
	}
	if err := json.Unmarshal([]byte(`{"T":""}`), &a); err != nil {
		t.Fatal(err)
	}
	if time.Duration(a.T).Seconds() != 0 {
		t.Fatal(a.T)
	}
}
