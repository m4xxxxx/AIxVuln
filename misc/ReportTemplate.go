package misc

import (
	"os"
	"path/filepath"
	"sync"
)

var reportTemplateOnce sync.Once

const defaultVerifierTemplate = `Runtime evidence is required to provide detailed and compelling proof of the vulnerability's existence. You will be given some verified runtime evidence, vulnerability information, and a proof of concept (PoC) that can be used directly. However, if the provided runtime evidence is insufficient or not "conclusive enough," you may independently conduct further testing and evidence collection in the runtime environment (specifically in the WebEnvInfo from the key information).
The report template is as follows:
# 厂商信息
- **漏洞厂商**：
- **厂商官网**：
- **影响产品**：
- **影响版本**：
- **报告撰写人**：
- **报告撰写时间**：
# 漏洞信息
- **漏洞名称**：
- **漏洞描述**：
- **临时解决方案**：
- **正式修复建议**：
# 漏洞分析
## 漏洞触发点
## 完整利用链分析
## 验证环境与运行证据
## HTTP请求与响应包（可选）
## POC
## POC运行结果
`

const defaultAnalyzeTemplate = `The report template is as follows:
# 漏洞信息
- **漏洞名称**：
- **漏洞描述**：
# 漏洞分析
## 漏洞触发点
## 完整利用链分析
## 验证方式`

func reportTemplateDir() string {
	return filepath.Join(GetDataDir(), ".reportTemplate")
}

func initReportTemplates() {
	reportTemplateOnce.Do(func() {
		dir := reportTemplateDir()
		_ = os.MkdirAll(dir, 0755)

		verifierPath := filepath.Join(dir, "verifier.md")
		if _, err := os.Stat(verifierPath); os.IsNotExist(err) {
			_ = os.WriteFile(verifierPath, []byte(defaultVerifierTemplate), 0644)
		}

		analyzePath := filepath.Join(dir, "analyze.md")
		if _, err := os.Stat(analyzePath); os.IsNotExist(err) {
			_ = os.WriteFile(analyzePath, []byte(defaultAnalyzeTemplate), 0644)
		}
	})
}

// GetReportTemplate reads a report template by name (e.g. "verifier", "analyze").
// Returns the file content, or the built-in default if the file cannot be read.
func GetReportTemplate(name string) string {
	initReportTemplates()
	path := filepath.Join(reportTemplateDir(), name+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		// Fallback to built-in defaults.
		switch name {
		case "verifier":
			return defaultVerifierTemplate
		case "analyze":
			return defaultAnalyzeTemplate
		default:
			return defaultAnalyzeTemplate
		}
	}
	return string(data)
}

// SetReportTemplate writes a report template by name.
func SetReportTemplate(name string, content string) error {
	initReportTemplates()
	path := filepath.Join(reportTemplateDir(), name+".md")
	return os.WriteFile(path, []byte(content), 0644)
}

// GetAllReportTemplates returns all report templates as a map of name -> content.
func GetAllReportTemplates() map[string]string {
	initReportTemplates()
	result := make(map[string]string)
	dir := reportTemplateDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		// Return built-in defaults.
		result["verifier"] = defaultVerifierTemplate
		result["analyze"] = defaultAnalyzeTemplate
		return result
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if len(name) > 3 && name[len(name)-3:] == ".md" {
			key := name[:len(name)-3]
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err == nil {
				result[key] = string(data)
			}
		}
	}
	// Ensure defaults exist.
	if _, ok := result["verifier"]; !ok {
		result["verifier"] = defaultVerifierTemplate
	}
	if _, ok := result["analyze"]; !ok {
		result["analyze"] = defaultAnalyzeTemplate
	}
	return result
}
