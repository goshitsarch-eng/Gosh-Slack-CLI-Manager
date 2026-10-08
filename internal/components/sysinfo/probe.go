package sysinfo

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"
)

// This file re-implements the subset of the Rust `sysinfo` 0.32 crate the
// dashboard uses, reading /proc, /sys and /etc the same way the crate does on
// Linux so that the displayed values match.

// minimumCPUUpdateInterval mirrors sysinfo::MINIMUM_CPU_UPDATE_INTERVAL: CPU
// times are only re-read when more than this has elapsed since the last read.
const minimumCPUUpdateInterval = 200 * time.Millisecond

// cpuValues holds the raw /proc/stat times of one CPU line.
type cpuValues struct {
	user, nice, system, idle, iowait, irq, softirq, steal, guest, guestNice uint64
}

func satAdd(a, b uint64) uint64 {
	if s := a + b; s >= a {
		return s
	}
	return ^uint64(0)
}

func satSub(a, b uint64) uint64 {
	if a > b {
		return a - b
	}
	return 0
}

func (v *cpuValues) set(f [10]uint64) {
	// guest is already accounted in user, guest_nice in nice.
	v.user = satSub(f[0], f[8])
	v.nice = satSub(f[1], f[9])
	v.system = f[2]
	v.idle = f[3]
	v.iowait = f[4]
	v.irq = f[5]
	v.softirq = f[6]
	v.steal = f[7]
	v.guest = f[8]
	v.guestNice = f[9]
}

func (v cpuValues) workTime() uint64 {
	return satAdd(satAdd(satAdd(satAdd(v.user, v.nice), v.system), v.irq), v.softirq)
}

func (v cpuValues) totalTime() uint64 {
	t := satAdd(v.workTime(), v.idle)
	t = satAdd(t, v.iowait)
	t = satAdd(t, v.guest)
	t = satAdd(t, v.guestNice)
	return satAdd(t, v.steal)
}

// cpuUsage mirrors sysinfo's CpuUsage.
type cpuUsage struct {
	percent      float32
	oldValues    cpuValues
	newValues    cpuValues
	totalTime    uint64
	oldTotalTime uint64
}

func newCPUUsage(f [10]uint64) cpuUsage {
	var u cpuUsage
	u.newValues.set(f)
	return u
}

func (u *cpuUsage) set(f [10]uint64) {
	u.oldValues = u.newValues
	u.newValues.set(f)
	u.totalTime = u.newValues.totalTime()
	u.oldTotalTime = u.oldValues.totalTime()
	work := float32(0)
	if nw, ow := u.newValues.workTime(), u.oldValues.workTime(); nw > ow {
		work = float32(nw - ow)
	}
	total := float32(1)
	if u.totalTime > u.oldTotalTime {
		total = float32(u.totalTime - u.oldTotalTime)
	}
	u.percent = work / total * 100
	if u.percent > 100 {
		u.percent = 100
	}
}

// toU64 mirrors sysinfo's unchecked digit accumulation (wrapping on bytes
// that are not digits, like the release build of the crate).
func toU64(b []byte) uint64 {
	var x uint64
	for _, c := range b {
		x = x*10 + uint64(c-'0')
	}
	return x
}

// cpuFields parses the up to ten numeric fields following a /proc/stat
// "cpu" line's name (missing ones are 0).
func cpuFields(parts [][]byte) [10]uint64 {
	var f [10]uint64
	for i := 0; i < 10 && i < len(parts); i++ {
		f[i] = toU64(parts[i])
	}
	return f
}

func splitNonEmpty(line []byte) [][]byte {
	var out [][]byte
	for _, p := range bytes.Split(line, []byte{' '}) {
		if len(p) > 0 {
			out = append(out, p)
		}
	}
	return out
}

// cpusWrapper mirrors sysinfo's CpusWrapper for the parts the dashboard uses.
type cpusWrapper struct {
	global     cpuUsage
	cpus       []cpuUsage
	lastUpdate time.Time
	hasUpdate  bool
}

// refresh mirrors CpusWrapper::refresh(false, CpuRefreshKind::everything()).
func (w *cpusWrapper) refresh() {
	if w.hasUpdate && time.Since(w.lastUpdate) <= minimumCPUUpdateInterval {
		return
	}
	first := len(w.cpus) == 0
	w.lastUpdate, w.hasUpdate = time.Now(), true
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return
	}
	lines := bytes.Split(data, []byte{'\n'})
	if len(lines) == 0 {
		return
	}
	if len(lines[0]) < 4 || string(lines[0][:4]) != "cpu " {
		return
	}
	w.global.set(cpuFields(splitNonEmpty(lines[0])[1:]))
	i := 0
	for _, line := range lines[1:] {
		if len(line) < 3 || string(line[:3]) != "cpu" {
			break
		}
		parts := splitNonEmpty(line)
		if len(parts) > 0 {
			parts = parts[1:] // the CPU name
		}
		if first {
			w.cpus = append(w.cpus, newCPUUsage(cpuFields(parts)))
		} else if i < len(w.cpus) {
			w.cpus[i].set(cpuFields(parts))
		}
		i++
	}
}

// memInfo mirrors the memory fields sysinfo reads from /proc/meminfo, in
// bytes.
type memInfo struct {
	total, free, available, buffers, pageCache, shmem, slabReclaimable uint64
	swapTotal, swapFree                                                uint64
}

func satMul(a, b uint64) uint64 {
	if a != 0 && a*b/a != b {
		return ^uint64(0)
	}
	return a * b
}

func (m *memInfo) refresh() {
	availableFound := false
	data, err := os.ReadFile("/proc/meminfo")
	if err == nil && utf8.Valid(data) {
		for _, line := range strings.Split(string(data), "\n") {
			key, rest, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			value, _, _ := strings.Cut(strings.TrimLeftFunc(rest, unicode.IsSpace), " ")
			v, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				continue
			}
			var field *uint64
			switch key {
			case "MemTotal":
				field = &m.total
			case "MemFree":
				field = &m.free
			case "MemAvailable":
				availableFound = true
				field = &m.available
			case "Buffers":
				field = &m.buffers
			case "Cached":
				field = &m.pageCache
			case "Shmem":
				field = &m.shmem
			case "SReclaimable":
				field = &m.slabReclaimable
			case "SwapTotal":
				field = &m.swapTotal
			case "SwapFree":
				field = &m.swapFree
			default:
				continue
			}
			*field = satMul(v, 1024)
		}
	}
	if !availableFound {
		m.available = satSub(satAdd(satAdd(satAdd(m.free, m.buffers), m.pageCache), m.slabReclaimable), m.shmem)
	}
}

func (m memInfo) usedMemory() uint64 { return m.total - m.available }
func (m memInfo) usedSwap() uint64   { return m.swapTotal - m.swapFree }

func utsField(f [65]int8) string {
	b := make([]byte, 0, len(f))
	for _, c := range f {
		if c == 0 {
			break
		}
		b = append(b, byte(c))
	}
	return string(b)
}

// hostName mirrors System::host_name (gethostname).
func hostName() (string, bool) {
	var u syscall.Utsname
	if syscall.Uname(&u) != nil {
		return "", false
	}
	s := utsField(u.Nodename)
	if !utf8.ValidString(s) {
		return "", false
	}
	return s, true
}

// kernelVersion mirrors System::kernel_version (uname release).
func kernelVersion() (string, bool) {
	var u syscall.Utsname
	if syscall.Uname(&u) != nil {
		return "", false
	}
	// The crate maps each byte to a char, which re-encodes bytes >= 0x80.
	var sb strings.Builder
	for _, c := range u.Release {
		if c != 0 {
			sb.WriteRune(rune(byte(c)))
		}
	}
	return sb.String(), true
}

// cpuArch mirrors System::cpu_arch (uname machine).
func cpuArch() (string, bool) {
	var u syscall.Utsname
	if syscall.Uname(&u) != nil {
		return "", false
	}
	s := utsField(u.Machine)
	if !utf8.ValidString(s) {
		return "", false
	}
	return s, true
}

// uptime mirrors System::uptime.
func uptime() uint64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil || !utf8.Valid(data) {
		return 0
	}
	first, _, _ := strings.Cut(string(data), ".")
	v, err := strconv.ParseUint(first, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// systemInfoLinux mirrors get_system_info_linux for the Name and OsVersion
// info types.
func systemInfoLinux(key, fallbackKey string) (string, bool) {
	if data, err := os.ReadFile("/etc/os-release"); err == nil && utf8.Valid(data) {
		for _, line := range rustStrLines(string(data)) {
			if v, ok := strings.CutPrefix(line, key); ok {
				return strings.ReplaceAll(v, `"`, ""), true
			}
		}
	}
	data, err := os.ReadFile("/etc/lsb-release")
	if err != nil || !utf8.Valid(data) {
		return "", false
	}
	for _, line := range rustStrLines(string(data)) {
		if v, ok := strings.CutPrefix(line, fallbackKey); ok {
			return strings.ReplaceAll(v, `"`, ""), true
		}
	}
	return "", false
}

func rustStrLines(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "\n")
	if parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	for i, p := range parts {
		parts[i] = strings.TrimSuffix(p, "\r")
	}
	return parts
}

// osName mirrors System::name.
func osName() (string, bool) { return systemInfoLinux("NAME=", "DISTRIB_ID=") }

// osVersion mirrors System::os_version.
func osVersion() (string, bool) { return systemInfoLinux("VERSION_ID=", "DISTRIB_RELEASE=") }

// countDisks mirrors Disks::new_with_refreshed_list().list().len().
func countDisks() int {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil || !utf8.Valid(data) {
		data = nil
	}
	n := 0
	for _, line := range rustStrLines(string(data)) {
		fields := strings.Fields(line)
		get := func(i int) string {
			if i < len(fields) {
				return fields[i]
			}
			return ""
		}
		fsSpec, fsFile, fsType := get(0), get(1), get(2)
		fsFile = strings.ReplaceAll(fsFile, `\134`, `\`)
		fsFile = strings.ReplaceAll(fsFile, `\040`, " ")
		fsFile = strings.ReplaceAll(fsFile, `\011`, "\t")
		fsFile = strings.ReplaceAll(fsFile, `\012`, "\n")
		switch fsType {
		case "rootfs", "sysfs", "proc", "devtmpfs", "cgroup", "cgroup2", "pstore",
			"squashfs", "rpc_pipefs", "iso9660", "tmpfs", "cifs", "nfs", "nfs4":
			continue
		}
		if strings.HasPrefix(fsFile, "/sys") || strings.HasPrefix(fsFile, "/proc") ||
			(strings.HasPrefix(fsFile, "/run") && !strings.HasPrefix(fsFile, "/run/media")) ||
			strings.HasPrefix(fsSpec, "sunrpc") {
			continue
		}
		var st syscall.Statfs_t
		var serr error
		for {
			serr = syscall.Statfs(fsFile, &st)
			if serr != syscall.EINTR {
				break
			}
		}
		if serr != nil {
			continue
		}
		if satMul(uint64(st.Bsize), st.Blocks) == 0 {
			continue
		}
		n++
	}
	return n
}

// netData mirrors the byte counters of sysinfo's NetworkData.
type netData struct {
	name           string
	rxBytes, oldRx uint64
	txBytes, oldTx uint64
}

func (d netData) received() uint64    { return satSub(d.rxBytes, d.oldRx) }
func (d netData) transmitted() uint64 { return satSub(d.txBytes, d.oldTx) }

// readCounter mirrors sysinfo's network `read`: one read of at most 30 bytes,
// parsing the leading digits.
func readCounter(path string) uint64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	buf := make([]byte, 30)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return 0
	}
	var ret uint64
	for i := 0; i < n && buf[i] >= '0' && buf[i] <= '9'; i++ {
		ret = ret*10 + uint64(buf[i]-'0')
	}
	return ret
}

// listNetworks mirrors Networks::new_with_refreshed_list. The crate keeps
// interfaces in a HashMap (random iteration order); they are kept sorted by
// name here for a deterministic display.
func listNetworks() []netData {
	entries, err := os.ReadDir("/sys/class/net/")
	if err != nil {
		return nil
	}
	var out []netData
	for _, e := range entries {
		name := e.Name()
		if !utf8.ValidString(name) {
			continue
		}
		stats := filepath.Join("/sys/class/net", name, "statistics")
		rx := readCounter(filepath.Join(stats, "rx_bytes"))
		tx := readCounter(filepath.Join(stats, "tx_bytes"))
		out = append(out, netData{name: name, rxBytes: rx, oldRx: rx, txBytes: tx, oldTx: tx})
	}
	return out
}

// refreshNetworks mirrors Networks::refresh.
func refreshNetworks(nets []netData) {
	for i := range nets {
		stats := filepath.Join("/sys/class/net", nets[i].name, "statistics")
		nets[i].oldRx, nets[i].rxBytes = nets[i].rxBytes, readCounter(filepath.Join(stats, "rx_bytes"))
		nets[i].oldTx, nets[i].txBytes = nets[i].txBytes, readCounter(filepath.Join(stats, "tx_bytes"))
	}
}

// process is the subset of sysinfo's Process the dashboard shows.
type process struct {
	pid      int
	name     string
	cpuUsage float32
	memory   uint64
}

// listProcesses mirrors a fresh System::new() followed by
// refresh_processes(ProcessesToUpdate::All, true): every process and thread
// (tasks listed under /proc/<pid>/task) with its resident memory. On a first
// refresh the crate has no previous CPU times, so every CPU usage is 0.
// The crate stores processes in a HashMap (random iteration order); they are
// returned sorted by PID here.
func listProcesses() []process {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	pageSize := uint64(os.Getpagesize())
	var out []process
	var visit func(dir, parentName string, e os.DirEntry)
	visit = func(dir, parentName string, e os.DirEntry) {
		if !e.IsDir() {
			return
		}
		name := e.Name()
		if parentName != "" && name == parentName {
			return
		}
		pid, err := strconv.ParseUint(name, 10, 64)
		if err != nil {
			return
		}
		path := filepath.Join(dir, name)
		if tasks, err := os.ReadDir(filepath.Join(path, "task")); err == nil {
			for _, t := range tasks {
				visit(filepath.Join(path, "task"), name, t)
			}
		}
		if p, ok := readProcess(path, int(pid), pageSize); ok {
			out = append(out, p)
		}
	}
	for _, e := range entries {
		visit("/proc", "", e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].pid < out[j].pid })
	return out
}

// readProcess mirrors _get_process_data for a process seen for the first
// time.
func readProcess(path string, pid int, pageSize uint64) (process, bool) {
	data, err := os.ReadFile(filepath.Join(path, "stat"))
	if err != nil {
		return process{}, false
	}
	first, rest, ok := bytes.Cut(data, []byte{' '})
	if !ok || !utf8.Valid(first) {
		return process{}, false
	}
	idx := bytes.LastIndexByte(rest, ')')
	if idx < 0 {
		return process{}, false
	}
	after := rest[idx+1:]
	if !utf8.Valid(after) {
		return process{}, false
	}
	shortExe := bytes.TrimPrefix(rest[:idx], []byte{'('})
	parts := append([]string{string(first)}, strings.Fields(string(after))...)
	const rssIndex = 22
	if len(parts) <= rssIndex {
		return process{}, false
	}
	p := process{pid: pid, name: strings.ToValidUTF8(string(shortExe), "�")}
	if statm, err := os.ReadFile(filepath.Join(path, "statm")); err == nil {
		fields := bytes.Split(statm, []byte{' '})
		if len(fields) > 1 {
			p.memory = satMul(toU64(fields[1]), pageSize)
		}
	} else {
		rss, _ := strconv.ParseUint(parts[rssIndex], 10, 64)
		p.memory = satMul(rss, pageSize)
	}
	return p, true
}

// sortByCPUDesc mirrors `sort_by(|a, b| b.cpu_usage().partial_cmp(&a.cpu_usage()))`.
func sortByCPUDesc(ps []process) {
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].cpuUsage > ps[j].cpuUsage })
}
