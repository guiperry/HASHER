package analyzer

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type Detective struct {
	IP     string
	ssh    *ssh.Client
	Model  string
	HasPRU bool
}

func NewDetective(ip, password string) (*Detective, error) {
	config := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	sshDetective, err := ssh.Dial("tcp", ip+":22", config)
	if err != nil {
		return nil, fmt.Errorf("SSH connection failed: %w", err)
	}

	detective := &Detective{
		IP:  ip,
		ssh: sshDetective,
	}

	// Detect model and capabilities
	if err := detective.detectCapabilities(); err != nil {
		sshDetective.Close()
		return nil, err
	}

	return detective, nil
}

func (c *Detective) detectCapabilities() error {
	// Check model
	output, err := c.RunCommand("cat /usr/bin/compile_time 2>/dev/null")
	if err == nil && output != "" {
		c.Model = strings.TrimSpace(output)
	}

	// Check for PRU
	output, err = c.RunCommand("ls -d /sys/devices/platform/ocp/4a300000.pruss* 2>/dev/null")
	c.HasPRU = (err == nil && output != "")

	return nil
}

func (c *Detective) RunCommand(cmd string) (string, error) {
	session, err := c.ssh.NewSession()
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return "", fmt.Errorf("command failed: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

func (c *Detective) UploadFile(localPath, remotePath string, content []byte) error {
	session, err := c.ssh.NewSession()
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	defer session.Close()

	// Create remote file
	cmd := fmt.Sprintf("cat > %s", remotePath)
	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}

	if err := session.Start(cmd); err != nil {
		return err
	}

	if _, err := stdin.Write(content); err != nil {
		return err
	}
	stdin.Close()

	return session.Wait()
}

func (c *Detective) StopMining() error {
	_, err := c.RunCommand("killall cgminer bmminer 2>/dev/null")
	return err
}

func (c *Detective) CheckPRU() (bool, error) {
	output, err := c.RunCommand(`
        if [ -d /sys/devices/platform/ocp/4a300000.pruss-soc-bus ]; then
            echo "PRU_AVAILABLE"
            ls /sys/devices/platform/ocp/4a300000.pruss-soc-bus/4a300000.pruss/
        else
            echo "PRU_NOT_FOUND"
        fi
    `)

	if err != nil {
		return false, err
	}

	return strings.Contains(output, "PRU_AVAILABLE"), nil
}

func (c *Detective) GetSystemInfo() (*SystemInfo, error) {
	info := &SystemInfo{}

	// Get CPU info
	output, _ := c.RunCommand("cat /proc/cpuinfo | grep Hardware | cut -d':' -f2")
	info.CPU = strings.TrimSpace(output)

	// Get memory
	output, _ = c.RunCommand("cat /proc/meminfo | grep MemTotal | awk '{print $2}'")
	fmt.Sscanf(output, "%d", &info.MemoryKB)

	// Get kernel version
	output, _ = c.RunCommand("uname -r")
	info.Kernel = strings.TrimSpace(output)

	// Get OS
	output, _ = c.RunCommand("cat /etc/openwrt_release | grep PRETTY_NAME | cut -d'\"' -f2")
	info.OS = strings.TrimSpace(output)

	return info, nil
}

func (c *Detective) Close() error {
	return c.ssh.Close()
}


