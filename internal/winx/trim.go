package winx

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var emptyWorkingSet = windows.NewLazySystemDLL("psapi.dll").NewProc("EmptyWorkingSet")

// TrimMemory hands the idle pages of this process and its children (the
// WebView2 processes) back to Windows. They move to the standby list and come
// back on demand, so a window that is hidden in the tray stops holding on to
// hundreds of MB it won't touch for hours. Failures are ignored: it is only a
// hint. It returns how many processes were trimmed.
func TrimMemory() int {
	n := 0
	for _, pid := range descendants(windows.GetCurrentProcessId()) {
		h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if err != nil {
			continue
		}
		if r, _, _ := emptyWorkingSet.Call(uintptr(h)); r != 0 {
			n++
		}
		windows.CloseHandle(h)
	}
	return n
}

// descendants lists root and every process below it.
func descendants(root uint32) []uint32 {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return []uint32{root}
	}
	defer windows.CloseHandle(snap)
	return processTree(root, parents(snap))
}

// parents maps each process ID in the snapshot to its parent's.
func parents(snap windows.Handle) map[uint32]uint32 {
	out := map[uint32]uint32{}
	e := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err := windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		out[e.ProcessID] = e.ParentProcessID
	}
	return out
}

// processTree returns root and all its descendants given each process's parent.
func processTree(root uint32, parent map[uint32]uint32) []uint32 {
	tree := []uint32{root}
	in := map[uint32]bool{root: true}
	for grew := true; grew; {
		grew = false
		for pid, ppid := range parent {
			if !in[pid] && in[ppid] {
				in[pid] = true
				tree = append(tree, pid)
				grew = true
			}
		}
	}
	return tree
}
