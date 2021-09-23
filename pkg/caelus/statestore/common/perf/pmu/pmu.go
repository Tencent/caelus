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

// +build linux

package pmu

/*
#include <stdlib.h>
#include <stdio.h>
#include <unistd.h>
#include <sys/syscall.h>
#include <string.h>
#include <sys/ioctl.h>
#include <linux/perf_event.h>
#include <linux/hw_breakpoint.h>
#include <asm/unistd.h>
#include <errno.h>
#include <stdint.h>
#include <inttypes.h>
#include <fcntl.h>

const int CPU_LIMITS = 256;

int llc_type = 0, llc_config = 0, llc_scale =  0;

struct read_format {
	uint64_t nr;
	struct {
		uint64_t value;
		//uint64_t id;
	} values[];
};

struct collectdatas {
	uint64_t instructions;
	uint64_t cycles;
	double cpi;
	uint64_t cachemisses;
	uint64_t cachereferences;
	uint64_t llcoccupancy;
};

static int open_read_event_file(char *path, char *buf) {
	int fd;

	if ((fd = open(path, O_RDONLY)) == -1) {
		fprintf(stderr, "failed to open %s: %d %s\n", path, errno, strerror(errno));
		return -1;
	}

	if (read(fd, buf, 1024) < 0) {
		fprintf(stderr, "failed to read %s: %d %s\n", path, errno, strerror(errno));
		return -1;
	}

	close(fd);
	//fprintf(stdout, "result: %s\n", buf);
	return 0;
}

int set_llc_config() {
	char tmp[1024];

	if (open_read_event_file("/sys/bus/event_source/devices/intel_cqm/type", tmp) != 0) {
		fprintf(stdout, "read /sys/bus/event_source/devices/intel_cqm/type err");
		return -1;
	}
	llc_type = atoi(tmp);

	memset(tmp, 0, sizeof(tmp));
	if (open_read_event_file("/sys/bus/event_source/devices/intel_cqm/events/llc_occupancy.scale", tmp) != 0) {
		fprintf(stdout, "read /sys/bus/event_source/devices/intel_cqm/events/llc_occupancy.scale err");
		return -1;
	}
	llc_scale = atoi(tmp);

	memset(tmp, 0, sizeof(tmp));
	if (open_read_event_file("/sys/bus/event_source/devices/intel_cqm/events/llc_occupancy", tmp) != 0) {
		fprintf(stdout, "read /sys/bus/event_source/devices/intel_cqm/events/llc_occupancy err");
		return -1;
	}

	char *token1 = strtok(tmp, ",");
	if (token1 != NULL) {
		char* token2 = strtok(token1, "=");
		token2 = strtok(NULL, "=");
		sscanf(token2, "%x", &llc_config);
	}

	return 0;
}

static long perf_event_open(struct perf_event_attr *hw_event, pid_t pid,
							int cpu, int group_fd, unsigned long flags)
{
	int ret;
	ret = syscall(__NR_perf_event_open, hw_event, pid, cpu, group_fd, flags);
	return ret;
}

int get_cpi(int interval, char* cgroup_path, char *cpu_str, struct collectdatas *output, int collect_llc) {
	int i = 0, cpu_len, llc_count = 0;
	struct perf_event_attr pea;
	int fd1, fd2, cfd;
	int64_t fds[CPU_LIMITS][10];
	char buf[4096];
	struct read_format* rf = (struct read_format*)buf;

	// parse core, input such as '2,3,5'
	int cpu_arr[256];
	char* token = strtok(cpu_str, ",");
	while (token != NULL) {
	if (strspn(token, "0123456789") == strlen(token)) {
		//fprintf(stdout, "token=%s\n", token);
		cpu_arr[i++] = atoi(token);
	}
	token = strtok(NULL, ",");
	}
	cpu_arr[i] = -1;
	cpu_len = i - 1;

	if ((cfd = open(cgroup_path, O_DIRECTORY|O_RDONLY)) == -1) {
		fprintf(stderr, "failed to open cgroup path(%s): %d %s\n", cgroup_path, errno, strerror(errno));
		return -1;
	}

	memset(output, 0,  sizeof(struct collectdatas));
	for (i = 0; i <= cpu_len; i++) {
		if (cpu_arr[i] == -1) {
			fprintf(stderr, "cpu id -1\n");
			return -1;
		}

		memset(&pea, 0, sizeof(struct perf_event_attr));
		pea.type = PERF_TYPE_HARDWARE;
		pea.size = sizeof(struct perf_event_attr);
		pea.config = PERF_COUNT_HW_CPU_CYCLES;
		pea.disabled = 1;
		pea.exclude_kernel = 1;
		pea.exclude_hv = 1;
		pea.read_format = PERF_FORMAT_GROUP;// | PERF_FORMAT_ID;
		fds[i][0] = perf_event_open(&pea, cfd, cpu_arr[i], -1, PERF_FLAG_PID_CGROUP);
		if (fds[i][0] < 0) {
			fprintf(stderr, "failed to open perf event for cpu cycles: %d %s\n", errno, strerror(errno));
			return -1;
		}

		pea.config = PERF_COUNT_HW_INSTRUCTIONS;
		pea.disabled = 0;
		fds[i][1] = perf_event_open(&pea, cfd, cpu_arr[i], fds[i][0], PERF_FLAG_PID_CGROUP);
		if (fds[i][1] < 0) {
			fprintf(stderr, "failed to open perf event for cpu instruction: %d %s\n", errno, strerror(errno));
			return -1;
		}

		pea.config = PERF_COUNT_HW_CACHE_REFERENCES;
		pea.disabled = 0;
		fds[i][2] = perf_event_open(&pea, cfd, cpu_arr[i], fds[i][0], PERF_FLAG_PID_CGROUP);
		if (fds[i][2] < 0) {
			fprintf(stderr, "failed to open perf event for cache ref: %d %s\n", errno, strerror(errno));
			return -1;
		}

		pea.config = PERF_COUNT_HW_CACHE_MISSES;
		pea.disabled = 0;
		fds[i][3] = perf_event_open(&pea, cfd, cpu_arr[i], fds[i][0], PERF_FLAG_PID_CGROUP);
		if (fds[i][3] < 0) {
			fprintf(stderr, "failed to open perf event for cache miss: %d %s\n", errno, strerror(errno));
			return -1;
		}

		if (collect_llc == 1) {
			memset(&pea, 0, sizeof(struct perf_event_attr));
			pea.type = llc_type; // /sys/bus/event_source/devices/intel_cqm/type
			pea.size = sizeof(struct perf_event_attr);
			pea.config = llc_config;
			pea.disabled = 1;
			//pea.pinned = 1;
			pea.read_format = PERF_FORMAT_GROUP;
			fds[i][4] = perf_event_open(&pea, cfd, cpu_arr[i], fds[i][0], PERF_FLAG_PID_CGROUP);
			if (fds[i][4] < 0) {
				fprintf(stderr, "failed to open perf event for llc occupancy: %d %s\n", errno, strerror(errno));
				return -1;
			}
		}
	}

	for (i = 0; i <= cpu_len; i++) {
		ioctl(fds[i][0], PERF_EVENT_IOC_RESET, PERF_IOC_FLAG_GROUP);
		ioctl(fds[i][0], PERF_EVENT_IOC_ENABLE, PERF_IOC_FLAG_GROUP);
	}

	sleep(interval);

	for (i = 0; i <= cpu_len; i++) {
		ioctl(fds[i][0], PERF_EVENT_IOC_DISABLE, PERF_IOC_FLAG_GROUP);
	}

	for (i = 0; i <= cpu_len; i++) {
		if (read(fds[i][0], buf, sizeof(buf)) == -1) {
			fprintf(stderr, "failed to read perf on cpu %d: %d %s\n", cpu_arr[i], errno, strerror(errno));
			return -1;
		}

		double cpi = 0.0;
		if (rf->values[1].value != 0) {
			cpi = (double)rf->values[0].value / rf->values[1].value;
		}

		output->instructions += rf->values[1].value;
		output->cycles += rf->values[0].value;
		output->cachereferences += rf->values[2].value;
		output->cachemisses += rf->values[3].value;
		close(fds[i][0]);
		close(fds[i][1]);
		close(fds[i][2]);
		close(fds[i][3]);

		uint64_t llc_new = 0;
		if (collect_llc == 1) {
			if (rf->values[4].value > 0) {
				llc_new = rf->values[4].value;
				if (llc_scale > 0) {
					llc_new *= llc_scale;
				}
				llc_count++;
			}
			output->llcoccupancy += llc_new;
			close(fds[i][4]);
		}

		//fprintf(stdout, "cpu:%d, cycles:%lld instr:%lld cpi:%f cacheref:%lld cachemisses:%lld llc:%lld\n",
		//	cpu_arr[i], rf->values[0].value, rf->values[1].value, cpi, rf->values[2].value,
		//	rf->values[3].value, llc_new);
	}

	if (llc_count != 0) {
		output->llcoccupancy = output->llcoccupancy / llc_count;
	}
	if (output->instructions != 0) {
		output->cpi = (double)output->cycles/output->instructions;
	}
	//fprintf(stdout, "cycles:%lld instr:%lld cpi:%f cacheref:%lld cachemisses:%lld llc:%lld\n",
	//        output->cycles, output->instructions, output->cpi, output->cachereferences, output->cachemisses,
	// 		output->llcoccupancy);
	close(cfd);
	return 0;
}

*/
import "C"
import (
	"os"
	"time"
	"unsafe"

	"k8s.io/klog"
)

const (
	intelCQMPath = "/sys/bus/event_source/devices/intel_cqm"
)

var (
	intelCQMChecked   = false
	intelCQMSupported = false
)

// PerfData group options for collecting perf data
type PerfData struct {
	Instructions   float64
	Cycles         float64
	CPI            float64
	CPUUsage       float64
	CacheMisses    float64
	CacheReference float64
	LLCOccupancy   float64
	Timestamp      time.Time
}

// GetPMUValue will call perf_event_open function and collect perf data
func GetPMUValue(period int, cgroupPath string, cpusets string) (PerfData, error) {
	data := PerfData{}

	var llc_collect = 0
	checkIntelCqmSupported()
	if intelCQMSupported {
		llc_collect = 1
	}

	cp := C.CString(cgroupPath)
	defer C.free(unsafe.Pointer(cp))
	css := C.CString(cpusets)
	defer C.free(unsafe.Pointer(css))

	var pc C.struct_collectdatas
	C.get_cpi(C.int(period), cp, css, (*C.struct_collectdatas)(unsafe.Pointer(&pc)), C.int(llc_collect))
	data.Timestamp = time.Now()
	data.Instructions = float64(C.ulong(pc.instructions))
	data.Cycles = float64(C.ulong(pc.cycles))
	//data.LLCOccupancy = float64(C.ulong(pc.llcoccupancy))
	data.LLCOccupancy = float64(int64(C.ulong(pc.llcoccupancy)/1024)) / 1024
	data.CacheReference = float64(C.ulong(pc.cachereferences))
	data.CacheMisses = float64(C.ulong(pc.cachemisses))
	data.CPI = float64(int(C.double(pc.cpi)*1000+0.5)) / 1000
	klog.V(4).Infof("collected perf(%s) data:%+v", cgroupPath, data)

	return data, nil
}

func checkIntelCqmSupported() bool {
	if intelCQMChecked {
		return intelCQMSupported
	}

	if _, err := os.Stat(intelCQMPath); err != nil {
		intelCQMSupported = false
	} else {
		intelCQMSupported = true
		ret := C.set_llc_config()
		if ret == -1 {
			intelCQMSupported = false
		}
	}

	intelCQMChecked = true
	return intelCQMSupported
}
