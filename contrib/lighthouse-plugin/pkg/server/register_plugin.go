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

package server

import (
	// init offline mutator
	_ "github.com/tencent/lighthouse-plugin/pkg/plugin/docker/offline"
	// init pid limit mutator
	_ "github.com/tencent/lighthouse-plugin/pkg/plugin/docker/pid-limit"
	// init storage opts mutator
	_ "github.com/tencent/lighthouse-plugin/pkg/plugin/docker/storage-opts"
	// init uts mode mutator
	_ "github.com/tencent/lighthouse-plugin/pkg/plugin/docker/uts-mode"
)
