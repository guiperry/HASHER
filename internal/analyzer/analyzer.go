package analyzer

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/crypto/ssh"
)

// Protocol represents the detected protocol of an ASIC device
type Protocol int

const (
	ProtocolUnknown Protocol = iota
	ProtocolBitmain
	ProtocolCGMiner
	ProtocolBMMiner
	ProtocolWhatsMiner
)

func (p Protocol) String() string {
	switch p {
	case ProtocolBitmain:
		return "Bitmain"
	case ProtocolCGMiner:
		return "CGMiner"
	case ProtocolBMMiner:
		return "BMMiner"
	case ProtocolWhatsMiner:
		return "WhatsMiner"
	default:
		return "Unknown"
	}
}

// DeviceInfo contains information about a detected ASIC device
type DeviceInfo struct {
	IPAddress   string            `json:"ip_address"`
	Hostname    string            `json:"hostname"`
	MACAddress  string            `json:"mac_address,omitempty"`
	DeviceType  string            `json:"device_type,omitempty"`
	OpenPorts   []int             `json:"open_ports"`
	Services    map[string]string `json:"services"`
	WebTitle    string            `json:"web_title,omitempty"`
	AuthMethods []string          `json:"auth_methods"`
	Accessible  bool              `json:"accessible"`
	Protocol    Protocol          `json:"protocol"`
}

// SystemInfo contains detailed system information about a device
type SystemInfo struct {
	CPU      string `json:"cpu"`
	MemoryKB int    `json:"memory_kb"`
	Kernel   string `json:"kernel"`
	OS       string `json:"os"`
	Model    string `json:"model"`
	HasPRU   bool   `json:"has_pru"`
}

// ScannerConfig holds configuration for network scanning
type ScannerConfig struct {
	Subnet          string        // CIDR notation, e.g., "192.168.1.0/24"
	Timeout         time.Duration // Connection timeout
	ConcurrentScans int           // Number of concurrent workers
	Username        string        // Known username for auth testing
	Password        string        // Known password for auth testing
	Ports           []int         // Ports to scan
}

// Default ports to scan for ASIC miners
var defaultPorts = []int{
	22,   // SSH
	23,   // Telnet (older devices)
	80,   // HTTP (web interface)
	443,  // HTTPS (secure web)
	8080, // Alternative HTTP
	4028, // CGMiner API
	4029, // BMMiner API (Antminer)
	3333, // Mining stratum (indicates miner)
	3334, // Alternative stratum
}

// Miner signatures for device type detection
var minerSignatures = []string{
	"antminer",
	"bmminer",
	"cgminer",
	"avalon",
	"whatsminer",
	"innosilicon",
	"goldshell",
}

// NewScannerConfig creates a default scanner configuration
func NewScannerConfig(subnet, password string) ScannerConfig {
	return ScannerConfig{
		Subnet:          subnet,
		Timeout:         2 * time.Second,
		ConcurrentScans: 50,
		Username:        "root",
		Password:        password,
		Ports:           defaultPorts,
	}
}

// ScanNetwork scans a network for ASIC devices
func ScanNetwork(config ScannerConfig) ([]DeviceInfo, error) {
	ip, ipnet, err := net.ParseCIDR(config.Subnet)
	if err != nil {
		return nil, err
	}

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, config.ConcurrentScans)
	results := make(chan DeviceInfo, 100)
	var mu sync.Mutex
	var devices []DeviceInfo

	var ips []string
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); incrementIP(ip) {
		ips = append(ips, ip.String())
	}

	for _, ipStr := range ips {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(ip string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			if device := scanHost(ip, config); device != nil {
				results <- *device
			}
		}(ipStr)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for device := range results {
		mu.Lock()
		devices = append(devices, device)
		mu.Unlock()
	}

	return devices, nil
}

func scanHost(ip string, config ScannerConfig) *DeviceInfo {
	device := &DeviceInfo{
		IPAddress:   ip,
		Services:    make(map[string]string),
		AuthMethods: []string{},
	}

	openPorts := scanPorts(ip, config.Ports, config.Timeout)
	if len(openPorts) == 0 {
		return nil
	}
	device.OpenPorts = openPorts

	for _, port := range openPorts {
		switch port {
		case 22:
			if checkSSH(ip, config, device) {
				device.Services["ssh"] = "OpenSSH"
			}
		case 23:
			if checkTelnet(ip, config, device) {
				device.Services["telnet"] = "Telnet"
			}
		case 80, 8080:
			checkHTTP(ip, port, config, device, false)
		case 443:
			checkHTTP(ip, port, config, device, true)
		case 4028, 4029:
			device.Services["cgminer-api"] = fmt.Sprintf("Port %d", port)
			device.DeviceType = guessDeviceType(port)
		}
	}

	if isMinerDevice(device) {
		device.Protocol = detectProtocol(device)
		return device
	}
	return nil
}

func scanPorts(ip string, ports []int, timeout time.Duration) []int {
	var openPorts []int
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, port := range ports {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			address := net.JoinHostPort(ip, fmt.Sprintf("%d", p))
			conn, err := net.DialTimeout("tcp", address, timeout)
			if err != nil {
				return
			}
			conn.Close()
			mu.Lock()
			openPorts = append(openPorts, p)
			mu.Unlock()
		}(port)
	}
	wg.Wait()
	return openPorts
}

func checkSSH(ip string, config ScannerConfig, device *DeviceInfo) bool {
	sshConfig := &ssh.ClientConfig{
		User: config.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(config.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         config.Timeout,
	}

	client, err := ssh.Dial("tcp", net.JoinHostPort(ip, "22"), sshConfig)
	if err != nil {
		if strings.Contains(err.Error(), "unable to authenticate") {
			device.AuthMethods = append(device.AuthMethods, "ssh-password")
			return true
		}
		return false
	}
	defer client.Close()

	device.Accessible = true
	device.AuthMethods = append(device.AuthMethods, "ssh-success")

	session, err := client.NewSession()
	if err == nil {
		output, _ := session.Output("hostname")
		if len(output) > 0 {
			device.Hostname = strings.TrimSpace(string(output))
		}
		session.Close()
	}

	return true
}

func checkTelnet(ip string, config ScannerConfig, device *DeviceInfo) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, "23"), config.Timeout)
	if err != nil {
		return false
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(config.Timeout))
	conn.SetWriteDeadline(time.Now().Add(config.Timeout))

	reader := bufio.NewReader(conn)
	banner, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false
	}

	if strings.Contains(strings.ToLower(banner), "login") {
		device.AuthMethods = append(device.AuthMethods, "telnet")

		fmt.Fprintf(conn, "%s\n", config.Username)
		time.Sleep(100 * time.Millisecond)
		fmt.Fprintf(conn, "%s\n", config.Password)
		time.Sleep(100 * time.Millisecond)

		conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		response, _ := reader.ReadString('$')
		if strings.Contains(response, "$") || strings.Contains(response, "#") {
			device.Accessible = true
		}
	}

	return true
}

func checkHTTP(ip string, port int, config ScannerConfig, device *DeviceInfo, isTLS bool) {
	scheme := "http"
	if isTLS {
		scheme = "https"
	}

	url := fmt.Sprintf("%s://%s/", scheme, net.JoinHostPort(ip, fmt.Sprintf("%d", port)))

	client := &http.Client{
		Timeout: config.Timeout,
		Transport: &http.Transport{
			TLSHandshakeTimeout:   config.Timeout,
			ResponseHeaderTimeout: config.Timeout,
		},
	}

	resp, err := client.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	bodyBytes := make([]byte, 2048)
	n, _ := io.ReadFull(resp.Body, bodyBytes)
	body := string(bodyBytes[:n])
	bodyLower := strings.ToLower(body)

	for _, sig := range minerSignatures {
		if strings.Contains(bodyLower, sig) {
			device.DeviceType = sig
			device.Services[scheme] = "ASIC Web Interface"
			break
		}
	}

	if title := extractTitle(body); title != "" {
		device.WebTitle = title
	}

	if resp.StatusCode == 401 {
		device.AuthMethods = append(device.AuthMethods, fmt.Sprintf("%s-basic-auth", scheme))
	} else if strings.Contains(bodyLower, "login") || strings.Contains(bodyLower, "password") {
		device.AuthMethods = append(device.AuthMethods, fmt.Sprintf("%s-form-auth", scheme))
	}

	if strings.Contains(bodyLower, "openwrt") || strings.Contains(bodyLower, "luci") {
		device.DeviceType = "OpenWrt"
		tryOpenWrtLogin(ip, port, scheme, config, device)
	}
}

func tryOpenWrtLogin(ip string, port int, scheme string, config ScannerConfig, device *DeviceInfo) {
	loginURL := fmt.Sprintf("%s://%s/cgi-bin/luci", scheme, net.JoinHostPort(ip, fmt.Sprintf("%d", port)))

	data := url.Values{}
	data.Set("luci_username", config.Username)
	data.Set("luci_password", config.Password)

	client := &http.Client{
		Timeout: config.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.PostForm(loginURL, data)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 302 || resp.StatusCode == 303 {
		location := resp.Header.Get("Location")
		if strings.Contains(location, "admin") || strings.Contains(location, "overview") {
			device.Accessible = true
			device.AuthMethods = append(device.AuthMethods, "openwrt-web-success")
		}
	}
}

func isMinerDevice(device *DeviceInfo) bool {
	hasMiningPort := false
	for _, port := range device.OpenPorts {
		if port == 4028 || port == 4029 || port == 3333 || port == 3334 {
			hasMiningPort = true
			break
		}
	}

	hasMinerSignature := device.DeviceType != ""

	hasWebInterface := false
	for svc := range device.Services {
		if svc == "http" || svc == "https" {
			hasWebInterface = true
			break
		}
	}

	return hasMiningPort || hasMinerSignature || (hasWebInterface && len(device.OpenPorts) <= 5)
}

func guessDeviceType(port int) string {
	switch port {
	case 4028:
		return "CGMiner"
	case 4029:
		return "BMMiner/Antminer"
	default:
		return "Unknown Miner"
	}
}

func extractTitle(html string) string {
	start := strings.Index(strings.ToLower(html), "<title>")
	if start == -1 {
		return ""
	}
	start += 7
	end := strings.Index(strings.ToLower(html)[start:], "</title>")
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(html[start : start+end])
}

func incrementIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func detectProtocol(device *DeviceInfo) Protocol {
	if strings.Contains(strings.ToLower(device.DeviceType), "antminer") ||
		strings.Contains(strings.ToLower(device.DeviceType), "bitmain") {
		return ProtocolBitmain
	}
	if strings.Contains(strings.ToLower(device.DeviceType), "cgminer") {
		return ProtocolCGMiner
	}
	if strings.Contains(strings.ToLower(device.DeviceType), "bmminer") {
		return ProtocolBMMiner
	}
	if strings.Contains(strings.ToLower(device.DeviceType), "whatsminer") {
		return ProtocolWhatsMiner
	}
	return ProtocolUnknown
}

// ProvisionDevice provisions a device with hasher-server
func ProvisionDevice(ip, password string) error {
	detective, err := NewDetective(ip, password)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer detective.Close()

	info, err := detective.GetSystemInfo()
	if err != nil {
		return fmt.Errorf("failed to get system info: %w", err)
	}

	fmt.Printf("Model: %s\n", info.Model)
	fmt.Printf("CPU: %s\n", info.CPU)
	fmt.Printf("Memory: %d MB\n", info.MemoryKB/1024)
	fmt.Printf("Kernel: %s\n", info.Kernel)
	fmt.Printf("OS: %s\n", info.OS)

	hasPRU, err := detective.CheckPRU()
	if err != nil {
		fmt.Printf("Warning: Could not check PRU: %v\n", err)
	} else if hasPRU {
		fmt.Println("PRU subsystem detected!")
	} else {
		fmt.Println("PRU subsystem not found")
	}

	if err := detective.StopMining(); err != nil {
		fmt.Printf("Warning: Failed to stop mining: %v\n", err)
	}

	return nil
}

// ProbeDevice probes a connected device for detailed information
func ProbeDevice() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("must run as root")
	}

	const DevicePath = "/dev/bitmain-asic"

	if !stopCGMiner() {
		fmt.Println("Warning: Could not confirm cgminer stopped")
	}

	if !checkDevice(DevicePath) {
		return fmt.Errorf("device not accessible")
	}

	device, err := openDevice(DevicePath)
	if err != nil {
		return fmt.Errorf("failed to open device: %w", err)
	}
	defer device.Close()

	probeDeviceInfo(device)
	probeReadOperations(device)
	probeWriteOperations(device)
	probeIOCTLs(device)

	return nil
}

func stopCGMiner() bool {
	cmd := exec.Command("pgrep", "cgminer")
	err := cmd.Run()
	if err != nil {
		fmt.Println("CGMiner is not running")
		return true
	}

	fmt.Println("Stopping CGMiner gracefully...")
	cmd = exec.Command("/etc/init.d/cgminer", "stop")
	cmd.Run()

	time.Sleep(5 * time.Second)

	cmd = exec.Command("pgrep", "cgminer")
	err = cmd.Run()
	if err != nil {
		fmt.Println("CGMiner stopped gracefully")
		return true
	}

	fmt.Println("Force killing CGMiner...")
	cmd = exec.Command("killall", "-9", "cgminer")
	cmd.Run()
	time.Sleep(2 * time.Second)

	cmd = exec.Command("pgrep", "cgminer")
	err = cmd.Run()
	if err != nil {
		fmt.Println("CGMiner stopped")
		return true
	}

	return false
}

func checkDevice(devicePath string) bool {
	info, err := os.Stat(devicePath)
	if err != nil {
		fmt.Printf("Device not found: %v\n", err)
		return false
	}

	fmt.Printf("Device: %s\n", devicePath)
	fmt.Printf("Mode: %s\n", info.Mode())
	fmt.Printf("Size: %d\n", info.Size())

	return true
}

func openDevice(devicePath string) (*os.File, error) {
	fmt.Println("Attempting to open with O_RDWR...")
	file, err := os.OpenFile(devicePath, os.O_RDWR, 0)
	if err != nil {
		fmt.Printf("O_RDWR failed: %v\n", err)

		fmt.Println("Attempting to open with O_RDONLY...")
		file, err = os.OpenFile(devicePath, os.O_RDONLY, 0)
		if err != nil {
			return nil, err
		}
		fmt.Println("Opened as read-only")
	} else {
		fmt.Println("Opened as read-write")
	}

	return file, nil
}

func probeDeviceInfo(device *os.File) {
	fmt.Println("Device Information:")
	fd := device.Fd()
	fmt.Printf("File Descriptor: %d\n", fd)
}

func probeReadOperations(device *os.File) {
	fmt.Println("Read Operations:")
	readWithTimeout(device, 64, 2*time.Second)
	readWithTimeout(device, 256, 2*time.Second)
	device.Seek(0, 0)
	readWithTimeout(device, 64, 2*time.Second)
}

func probeWriteOperations(device *os.File) {
	fmt.Println("Write Operations:")
	testData := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}
	n, err := device.Write(testData)
	if err != nil {
		fmt.Printf("Write error: %v\n", err)
	} else {
		fmt.Printf("Wrote %d bytes: %s\n", n, hex.EncodeToString(testData))
	}
}

func probeIOCTLs(device *os.File) {
	fmt.Println("IOCTL Discovery:")
	
	fd := device.Fd()
	fmt.Printf("File Descriptor: %d\n", fd)
	
	// Common ASIC miner IOCTL commands to probe
	// These are typical IOCTL commands for Bitmain/Antminer devices
	ioctlCommands := []struct {
		name string
		cmd  uintptr
	}{
		{"GET_CHIP_STATUS", 0x4000 | 0x100},
		{"GET_HASHRATE", 0x4000 | 0x101},
		{"GET_TEMPERATURE", 0x4000 | 0x102},
		{"GET_FAN_SPEED", 0x4000 | 0x103},
		{"GET_VOLTAGE", 0x4000 | 0x104},
		{"SET_FREQUENCY", 0x8000 | 0x200},
		{"SET_VOLTAGE", 0x8000 | 0x201},
		{"RESET_CHIP", 0x8000 | 0x202},
		{"ENABLE_CHIP", 0x8000 | 0x203},
		{"DISABLE_CHIP", 0x8000 | 0x204},
	}
	
	for _, ioctl := range ioctlCommands {
		// Try to probe the IOCTL command
		// We use a dummy buffer for testing
		var buf [256]byte
		_, _, errno := syscall.Syscall(
			syscall.SYS_IOCTL,
			fd,
			ioctl.cmd,
			uintptr(unsafe.Pointer(&buf[0])),
		)
		
		switch errno {
		case 0:
			fmt.Printf("  ✓ %s (0x%x) - Supported\n", ioctl.name, ioctl.cmd)
		case syscall.ENOTTY, syscall.EINVAL:
			fmt.Printf("  ✗ %s (0x%x) - Not supported\n", ioctl.name, ioctl.cmd)
		default:
			fmt.Printf("  ? %s (0x%x) - Error: %v\n", ioctl.name, ioctl.cmd, errno)
		}
	}
	
	// Try to get device information via generic IOCTLs
	fmt.Println("\nGeneric Device Information:")
	
	// Try TIOCGWINSZ to see if it's a terminal
	var ws struct {
		Row    uint16
		Col    uint16
		Xpixel uint16
		Ypixel uint16
	}
	
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		fd,
		syscall.TIOCGWINSZ,
		uintptr(unsafe.Pointer(&ws)),
	)
	
	if errno == 0 {
		fmt.Printf("  Terminal size: %dx%d\n", ws.Col, ws.Row)
	}
	
	// Get file status using fstat
	var stat syscall.Stat_t
	err := syscall.Fstat(int(fd), &stat)
	if err == nil {
		fmt.Printf("  Device ID: %d,%d\n", stat.Dev, stat.Rdev)
		fmt.Printf("  Inode: %d\n", stat.Ino)
		fmt.Printf("  Mode: 0%o\n", stat.Mode)
		
		// Check device type
		if stat.Mode&syscall.S_IFCHR != 0 {
			fmt.Println("  Type: Character device")
		} else if stat.Mode&syscall.S_IFBLK != 0 {
			fmt.Println("  Type: Block device")
		}
	}
}

func readWithTimeout(device *os.File, size int, timeout time.Duration) {
	done := make(chan bool)
	buffer := make([]byte, size)
	var n int
	var err error

	go func() {
		n, err = device.Read(buffer)
		done <- true
	}()

	select {
	case <-done:
		if err != nil {
			fmt.Printf("Read error: %v\n", err)
		} else if n == 0 {
			fmt.Println("Read 0 bytes")
		} else {
			fmt.Printf("Read %d bytes: %s\n", n, hex.EncodeToString(buffer[:n]))
		}
	case <-time.After(timeout):
		fmt.Println("Read timeout")
	}
}

// ProtocolDiscover discovers the communication protocol used by the device
func ProtocolDiscover() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("must run as root")
	}

	const DevicePath = "/dev/bitmain-asic"

	stopCGMiner()

	device, err := os.OpenFile(DevicePath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("failed to open device: %w", err)
	}
	defer device.Close()

	testPatterns := []struct {
		name string
		data []byte
		desc string
	}{
		{
			name: "NULL_COMMAND",
			data: []byte{0x00},
			desc: "Single null byte",
		},
		{
			name: "MAGIC_HEADER",
			data: []byte{0x55, 0xAA},
			desc: "Common UART magic bytes",
		},
		{
			name: "INIT_SEQUENCE",
			data: []byte{0x55, 0xAA, 0x00, 0x00},
			desc: "Possible initialization",
		},
		{
			name: "STATUS_REQUEST",
			data: []byte{0x01, 0x00, 0x00, 0x00},
			desc: "Status query",
		},
		{
			name: "VERSION_REQUEST",
			data: []byte{0x02, 0x00, 0x00, 0x00},
			desc: "Version query",
		},
	}

	for i, test := range testPatterns {
		fmt.Printf("[%d/%d] %s\n", i+1, len(testPatterns), test.name)
		fmt.Printf("Description: %s\n", test.desc)
		fmt.Printf("Data: %s\n", hex.EncodeToString(test.data))

		n, err := device.Write(test.data)
		if err != nil {
			fmt.Printf("Write failed: %v\n", err)
			continue
		}
		fmt.Printf("Wrote %d bytes\n", n)

		response := make([]byte, 1024)
		device.SetReadDeadline(time.Now().Add(500 * time.Millisecond))

		n, err = device.Read(response)
		if err != nil {
			if os.IsTimeout(err) {
				fmt.Println("No response (timeout)")
			} else {
				fmt.Printf("Read error: %v\n", err)
			}
		} else if n > 0 {
			fmt.Printf("Response: %s\n", hex.Dump(response[:n]))
		} else {
			fmt.Println("Read 0 bytes")
		}

		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

// TestDevice tests the ASIC communication pattern
func TestDevice() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("must run as root")
	}

	if !checkEnvironment() {
		return fmt.Errorf("must run on MIPS architecture")
	}

	phase1_SystemInfo()
	phase2_KernelModules()
	phase3_DeviceInterface()
	phase4_ProtocolDiscovery()
	phase5_CGMinerAnalysis()
	phase6_DeviceTest()

	return nil
}

func checkEnvironment() bool {
	output, err := runCommand("uname", "-m")
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(output), "mips")
}

func phase1_SystemInfo() {
	fmt.Println("Phase 1: System Information")
	cpuInfo, err := runCommand("cat", "/proc/cpuinfo")
	if err == nil {
		printRelevantLines(cpuInfo, []string{"system type", "cpu model", "BogoMIPS"})
	}

	memInfo, err := runCommand("cat", "/proc/meminfo")
	if err == nil {
		printRelevantLines(memInfo, []string{"MemTotal", "MemFree", "MemAvailable"})
	}

	usbInfo, err := runCommand("lsusb")
	if err == nil {
		fmt.Println("USB Devices:")
		fmt.Println(usbInfo)
	}
}

func phase2_KernelModules() {
	fmt.Println("Phase 2: Kernel Modules")
	lsmodOutput, err := runCommand("lsmod")
	if err == nil {
		printRelevantLines(lsmodOutput, []string{"bitmain"})
	}
}

func phase3_DeviceInterface() {
	fmt.Println("Phase 3: Device Interface")
	const devicePath = "/dev/bitmain-asic"
	info, err := runCommand("ls", "-l", devicePath)
	if err == nil {
		fmt.Println(info)
	}

	sysfsInfo, err := runCommand("ls", "-la", "/sys/class/misc/bitmain-asic/")
	if err == nil {
		fmt.Println(sysfsInfo)
	}
}

func phase4_ProtocolDiscovery() {
	fmt.Println("Phase 4: Protocol Discovery")
	dmesgOutput, err := runCommand("dmesg")
	if err == nil {
		lines := strings.Split(dmesgOutput, "\n")
		var asicLines []string
		for _, line := range lines {
			if strings.Contains(strings.ToLower(line), "bitmain") ||
				strings.Contains(strings.ToLower(line), "asic") {
				asicLines = append(asicLines, line)
			}
		}

		start := 0
		if len(asicLines) > 20 {
			start = len(asicLines) - 20
		}
		for _, line := range asicLines[start:] {
			fmt.Println(line)
		}
	}
}

func phase5_CGMinerAnalysis() {
	fmt.Println("Phase 5: CGMiner Analysis")
	cgminerPath := "/usr/bin/cgminer"
	if _, err := os.Stat(cgminerPath); err == nil {
		info, err := runCommand("ls", "-lh", cgminerPath)
		if err == nil {
			fmt.Println(info)
		}
	}

	psOutput, err := runCommand("ps", "w")
	if err == nil {
		printRelevantLines(psOutput, []string{"cgminer"})
	}
}

func phase6_DeviceTest() {
	fmt.Println("Phase 6: Device Access Test")
	const devicePath = "/dev/bitmain-asic"
	file, err := os.OpenFile(devicePath, os.O_RDWR, 0)
	if err != nil {
		fmt.Printf("Cannot open device: %v\n", err)
		return
	}
	defer file.Close()

	fmt.Println("Device opened successfully!")
	fmt.Println("Attempting Read (3 second timeout):")
	readBuf := make([]byte, 64)
	readDone := make(chan bool)

	go func() {
		n, err := file.Read(readBuf)
		if err != nil {
			fmt.Printf("Read error: %v\n", err)
		} else if n > 0 {
			fmt.Printf("Read %d bytes: %x\n", n, readBuf[:n])
		} else {
			fmt.Println("Read 0 bytes")
		}
		readDone <- true
	}()

	select {
	case <-readDone:
	case <-time.After(3 * time.Second):
		fmt.Println("Read timeout")
	}
}

func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func printRelevantLines(output string, keywords []string) {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		for _, keyword := range keywords {
			if strings.Contains(strings.ToLower(line), strings.ToLower(keyword)) {
				fmt.Println(line)
				break
			}
		}
	}
}
