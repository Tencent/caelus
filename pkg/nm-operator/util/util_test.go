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

package util

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestExecuteYarnCMD(t *testing.T) {
	user := "tester"
	binFile := "/tmp/test.sh"

	// add user
	output, err := exec.Command("useradd", user).Output()
	if err != nil {
		t.Skipf("create user(%s) err: %v, output: %s, just skip", user, err, string(output))
	}
	defer exec.Command("userdel", "-rf", user).Output()

	// get uid & gid
	uidBytes, err := exec.Command("id", "-u", user).Output()
	if err != nil {
		t.Skipf("get user id fail for %s: %v, just skip", user, err)
	}
	uidStr := strings.Trim(string(uidBytes), "\n")
	gidBytes, err := exec.Command("id", "-g", user).Output()
	if err != nil {
		t.Skipf("get group id fail for %s: %v, just skip", user, err)
	}
	gidStr := strings.Trim(string(gidBytes), "\n")

	// generate executable file
	context := fmt.Sprintf(`
#!/bin/sh

if [ $# != 2 ]; then
   echo "should be two parameters"
   exit 1
fi

uid=$(id -u)
if [ "$uid" !=	"%s" ]; then
   echo "bad uid $uid"
   exit 1
fi

gid=$(id -g)
if [ "$gid" !=	"%s" ]; then
   echo "bad gid $gid"
   exit 1
fi

`, uidStr, gidStr)

	ioutil.WriteFile(binFile, []byte(context), 0755)
	os.Chmod(binFile, 0777)
	defer os.Remove(binFile)

	err = ExecuteYarnCMD([]string{binFile, "1", "2"}, user)
	if err != nil {
		t.Fatalf("execute yarn cmd err: %v", err)
	}
}
