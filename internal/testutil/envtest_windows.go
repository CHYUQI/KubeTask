//go:build windows

package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func stopEnvTest(env *envtest.Environment) error {
	killEnvTestChildren()
	return retryStop(env, 200*time.Millisecond, 30*time.Second)
}

// KillOrphanedEnvTestProcesses terminates etcd / kube-apiserver left behind by
// aborted test runs. It only targets processes whose parent process is gone
// and whose executable lives under one of assetDirs (or KUBEBUILDER_ASSETS),
// so processes owned by a live test run or a real local cluster are untouched.
func KillOrphanedEnvTestProcesses(assetDirs ...string) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return
	}
	defer windows.CloseHandle(snapshot)

	livePIDs := make(map[uint32]struct{})
	var envTestEntries []windows.ProcessEntry32
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err := windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		livePIDs[entry.ProcessID] = struct{}{}
		if !isEnvTestProcess(windows.UTF16ToString(entry.ExeFile[:])) {
			continue
		}
		envTestEntries = append(envTestEntries, entry)
	}

	dirs := append([]string{os.Getenv("KUBEBUILDER_ASSETS")}, assetDirs...)
	for _, entry := range envTestEntries {
		if _, alive := livePIDs[entry.ParentProcessID]; alive {
			continue // still owned by a live process, e.g. a parallel package test
		}
		if !isUnderAssetDirs(processImagePath(entry.ProcessID), dirs) {
			continue
		}
		_ = terminateProcess(entry.ProcessID)
	}
}

// killEnvTestChildren force-terminates envtest's etcd / kube-apiserver, which
// are direct children of the current test binary. controller-runtime cannot
// stop them on Windows because its Stop() relies on SIGTERM.
func killEnvTestChildren() {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return
	}
	defer windows.CloseHandle(snapshot)

	parentPID := uint32(os.Getpid())
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err := windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if entry.ParentProcessID != parentPID {
			continue
		}
		if !isEnvTestProcess(windows.UTF16ToString(entry.ExeFile[:])) {
			continue
		}
		_ = terminateProcess(entry.ProcessID)
	}
}

func isEnvTestProcess(name string) bool {
	switch strings.ToLower(name) {
	case "etcd.exe", "kube-apiserver.exe":
		return true
	}
	return false
}

func terminateProcess(pid uint32) error {
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	return windows.TerminateProcess(handle, 1)
}

func processImagePath(pid uint32) string {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(handle)

	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(handle, 0, &buf[0], &size); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:size])
}

func isUnderAssetDirs(path string, dirs []string) bool {
	if path == "" {
		return false
	}
	cleanPath := strings.ToLower(filepath.Clean(path))
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		absDir, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		cleanDir := strings.ToLower(filepath.Clean(absDir))
		if cleanPath == cleanDir || strings.HasPrefix(cleanPath, cleanDir+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
