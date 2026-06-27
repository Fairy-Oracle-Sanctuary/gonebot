// Package deps: venv 管理 (阶段 2).
//
// 职责:
//   - 创建 plugins_py/<name>/venv 虚拟环境
//   - 在 venv 中 pip install 依赖
//   - 复用 venv (基于依赖 hash)
package deps

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// VenvInfo venv 元信息.
type VenvInfo struct {
	PluginName string
	Path       string // 绝对路径
	PythonBin  string // venv 内 python 可执行
	Created    time.Time
	ReqsHash   string
	AccumReqs  []string // 累积的所有 reqs (共享 venv 用)
}

// EnsureVenv 确保 venv 可用并安装指定依赖, 返回 venv 内 python 路径.
//
// venvPath: 自定义 venv 路径 (留空则使用 pluginDir/venv, 每插件独立;
//           指定共享路径则所有插件共用, 节省空间).
//
// 流程:
//   1. 检查 venvPath 是否存在且 hash 匹配
//   2. 不存在则 python -m venv <venvPath>
//   3. 合并 reqs 到 venv/.nb_reqs.txt (累积所有插件需求)
//   4. hash 未变则跳过 pip install
//   5. 在 venv 内 pip install -r .nb_reqs.txt
func (m *Manager) EnsureVenv(venvPath, pluginName string, reqs []string) (*VenvInfo, error) {
	if !m.cfg.Enabled {
		return nil, nil
	}

	// 默认 venv 路径 = pluginDir/venv
	if venvPath == "" {
		// 该函数由调用方传入 pluginDir 转换, 这里保持兼容
		venvPath = filepath.Join(pluginName, "venv")
	}

	infoPath := filepath.Join(venvPath, ".nb_venv.json")
	reqFile := filepath.Join(venvPath, ".nb_reqs.txt")

	// 读取已累积的依赖 (共享 venv 模式下累积, 独立模式下被覆盖)
	existingReqs, existingHash := m.loadVenvAccumulated(infoPath)

	// 合并 reqs (去重, 保留顺序)
	merged := mergeReqs(existingReqs, reqs)
	mergedHash := hashReqs(merged)

	// 检查现有 venv 是否可用 (只比较 merged hash, 因为我们要安装的是全部需求)
	if info, err := m.loadVenvInfo(infoPath); err == nil {
		if info.ReqsHash == mergedHash && fileExists(filepath.Join(venvPath, "pyvenv.cfg")) {
			m.logger.Debug("venv already up-to-date", "plugin", pluginName, "path", venvPath)
			return info, nil
		}
	}

	// 创建 venv (如果不存在)
	if !fileExists(filepath.Join(venvPath, "pyvenv.cfg")) {
		m.logger.Info("creating venv", "plugin", pluginName, "path", venvPath)
		if err := m.createVenv(venvPath); err != nil {
			return nil, fmt.Errorf("create venv: %w", err)
		}
	}

	// 写累积依赖到 venv 内
	if err := os.WriteFile(reqFile, []byte(joinReqs(merged)+"\n"), 0o644); err != nil {
		return nil, fmt.Errorf("write reqs: %w", err)
	}

	// pip install
	pythonBin := m.venvPython(venvPath)
	if err := m.pipInstallInVenv(pythonBin, reqFile); err != nil {
		return nil, fmt.Errorf("pip install in venv: %w", err)
	}

	info := &VenvInfo{
		PluginName: pluginName,
		Path:       venvPath,
		PythonBin:  pythonBin,
		Created:    time.Now(),
		ReqsHash:   mergedHash,
	}
	if err := m.saveVenvInfo(infoPath, info); err != nil {
		m.logger.Warn("save venv info failed", "err", err.Error())
	}

	if existingHash != mergedHash {
		m.logger.Info("venv updated",
			"plugin", pluginName, "path", venvPath,
			"reqs_total", len(merged), "added", len(reqs))
	}
	return info, nil
}

// mergeReqs 合并两个 reqs 列表 (去重, 保留首次出现的).
func mergeReqs(a, b []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(a)+len(b))
	for _, r := range a {
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	for _, r := range b {
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	return out
}

// loadVenvAccumulated 从 venv info 读取已累积的依赖.
func (m *Manager) loadVenvAccumulated(infoPath string) ([]string, string) {
	data, err := os.ReadFile(infoPath)
	if err != nil {
		return nil, ""
	}
	// 反序列化 reqs (我们复用 VenvInfo 增加一个字段)
	var raw struct {
		ReqsHash    string   `json:"ReqsHash"`
		AccumReqs   []string `json:"AccumReqs"`
	}
	if err := jsonUnmarshal(data, &raw); err != nil {
		return nil, ""
	}
	return raw.AccumReqs, raw.ReqsHash
}

// createVenv 创建 venv: 有 uv 则用 uv venv, 否则回退 python -m venv.
func (m *Manager) createVenv(venvPath string) error {
	// 先删除旧 venv
	_ = os.RemoveAll(venvPath)

	// 优先尝试 uv (更快、更可靠)
	if uvPath, ok := findCommand("uv"); ok {
		m.logger.Debug("creating venv with uv", "path", venvPath)
		cmd := exec.Command(uvPath, "venv", "--python", m.cfg.PythonBin, venvPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			m.logger.Warn("uv venv failed, falling back to python -m venv", "err", string(output))
			_ = os.RemoveAll(venvPath)
		} else {
			return nil
		}
	}

	// 回退: python -m venv
	m.logger.Debug("creating venv with python -m venv", "path", venvPath)
	cmd := exec.Command(m.cfg.PythonBin, "-m", "venv", venvPath)
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("venv create failed: %w\noutput: %s", err, string(output))
	}
	return nil
}

// pipInstallInVenv 在 venv 内安装依赖: 有 uv 则用 uv pip install, 否则回退 pip.
func (m *Manager) pipInstallInVenv(pythonBin, reqFile string) error {
	// 优先尝试 uv pip install (解析和下载并行, 更快)
	if uvPath, ok := findCommand("uv"); ok {
		args := []string{"pip", "install",
			"--python", pythonBin,
		}
		if m.cfg.PipIndex != "" {
			args = append(args, "--index-url", m.cfg.PipIndex)
		}
		args = append(args, m.cfg.PipExtraArgs...)
		args = append(args, "-r", reqFile)

		cmd := exec.Command(uvPath, args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			m.logger.Warn("uv pip install failed, falling back to pip", "err", string(output))
		} else {
			return nil
		}
	}

	// 回退: python -m pip install
	args := []string{"-m", "pip", "install",
		"--disable-pip-version-check",
		"--no-warn-script-location",
	}
	if m.cfg.PipIndex != "" {
		args = append(args, "-i", m.cfg.PipIndex)
	}
	args = append(args, m.cfg.PipExtraArgs...)
	args = append(args, "-r", reqFile)

	cmd := exec.Command(pythonBin, args...)
	cmd.Env = append(os.Environ(), "PIP_DISABLE_PIP_VERSION_CHECK=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pip install failed: %w\noutput: %s", err, string(output))
	}
	return nil
}

// venvPython 返回 venv 内 python 可执行文件路径 (跨平台).
func (m *Manager) venvPython(venvPath string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(venvPath, "Scripts", "python.exe")
	}
	return filepath.Join(venvPath, "bin", "python")
}

// loadVenvInfo / saveVenvInfo venv 元信息持久化.
func (m *Manager) loadVenvInfo(path string) (*VenvInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var info VenvInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (m *Manager) saveVenvInfo(path string, info *VenvInfo) error {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func joinReqs(reqs []string) string {
	out := ""
	for i, r := range reqs {
		if i > 0 {
			out += "\n"
		}
		out += r
	}
	return out
}

// findCommand 在 PATH 中查找可执行文件, 返回路径和是否找到.
func findCommand(name string) (string, bool) {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path, err := exec.LookPath(name)
	return path, err == nil
}

// VenvSitePackages 返回 venv 的 site-packages 路径 (供 Python 进程 PYTHONPATH 使用).
func (m *Manager) VenvSitePackages(venvPath string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(venvPath, "Lib", "site-packages")
	}
	return filepath.Join(venvPath, "lib", fmt.Sprintf("python%s.%s", pythonVersion(), pythonVersion()), "site-packages")
}

func pythonVersion() string {
	// 简化: 不实际获取版本, 由 ensure venv 时返回精确路径
	// 这里只返回占位, 实际使用时建议探测
	return "3.10"
}

var _ = errors.New
var _ = fileExists