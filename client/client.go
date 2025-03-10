package client

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
	"runtime"
	"bufio"
	"encoding/binary"
	"path/filepath"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/schollz/progressbar/v3"
)

// OSSConfig OSS配置信息
type OSSConfig struct {
	Endpoint    string // OSS终端节点
	BucketName  string // OSS存储桶名称
	Credentials interface{} // OSS凭证信息
}

// OSSCredentials OSS临时凭证信息
type OSSCredentials struct {
	AccessKeyId     string `json:"access_key_id"`     // 临时访问密钥ID
	AccessKeySecret string `json:"access_key_secret"` // 临时访问密钥密码
	SecurityToken   string `json:"security_token"`    // 安全令牌
	Expiration     string `json:"expiration"`        // 过期时间
}

// OSSTokenResponse OSS临时凭证响应
type OSSTokenResponse struct {
	Code    int    `json:"code"`    // 响应状态码
	Success bool   `json:"success"` // 请求是否成功
	Message string `json:"message"` // 响应消息
	Data    struct {
		AccessKeyId     string `json:"AccessKeyId"`     // 临时访问密钥ID
		AccessKeySecret string `json:"AccessKeySecret"` // 临时访问密钥密码
		SecurityToken   string `json:"SecurityToken"`   // 安全令牌
		Expiration     string `json:"Expiration"`      // 过期时间
	} `json:"data"` // 凭证信息
}

// UploadFileResponse 上传文件响应
type UploadFileResponse struct {
	Success bool   `json:"success"` // 上传是否成功
	Message string `json:"message"` // 响应消息
	URL     string `json:"url"`     // 文件访问URL
}

// Client 表示 VoiceWorld API 客户端
type Client struct {
	appKey     string        // 应用密钥
	appSecret  string        // 应用密钥
	baseURL    string        // API基础URL地址
	httpClient *http.Client  // HTTP客户端实例
	ossConfig  *OSSConfig    // OSS配置信息
}

// ClientConfig 客户端配置选项
type ClientConfig struct {
	BaseURL    string     // API基础URL地址
	OSSConfig  *OSSConfig // OSS配置信息（可选）
}

// DefaultConfig 返回默认配置
func DefaultConfig() *ClientConfig {
	return &ClientConfig{
		BaseURL: "http://localhost:8000",
		OSSConfig: &OSSConfig{
			Endpoint:    "https://oss-cn-shanghai.aliyuncs.com",
			BucketName:  "voiceworld",
			Credentials: nil,
		},
	}
}

// NewClient 创建一个新的 VoiceWorld API 客户端实例
// appKey: 应用密钥
// appSecret: 应用密钥
// config: 客户端配置（可选，如果不提供则使用默认配置）
func NewClient(appKey, appSecret string, config ...*ClientConfig) *Client {
	cfg := DefaultConfig()
	if len(config) > 0 && config[0] != nil {
		if config[0].BaseURL != "" {
			cfg.BaseURL = config[0].BaseURL
		}
		if config[0].OSSConfig != nil {
			cfg.OSSConfig = config[0].OSSConfig
		}
	}

	return &Client{
		appKey:     appKey,
		appSecret:  appSecret,
		baseURL:    cfg.BaseURL,
		httpClient: &http.Client{},
		ossConfig:  cfg.OSSConfig,
	}
}

// GetOSSConfig 获取当前的 OSS 配置
func (c *Client) GetOSSConfig() *OSSConfig {
	return c.ossConfig
}

// SetOSSConfig 设置 OSS 配置
func (c *Client) SetOSSConfig(config *OSSConfig) {
	c.ossConfig = config
}

// ASRRequest 语音识别请求参数结构体
type ASRRequest struct {
	Format              string `json:"format"`               // 音频格式（如 "pcm"、"wav"）
	SampleRate          int    `json:"sample_rate"`         // 采样率（Hz）
	EnablePunctuation   bool   `json:"enable_punctuation"`  // 是否启用标点符号预测
	EnableNormalization bool   `json:"enable_normalization"` // 是否启用文本正规化
	TaskID             string `json:"task_id"`              // 任务ID，用于跟踪识别任务
}

// ASRResponse 语音识别响应结构体
type ASRResponse struct {
	Success bool   `json:"success"` // 识别是否成功
	Message string `json:"message"` // 响应消息
	Result  string `json:"result"`  // 识别结果文本
	TaskID  string `json:"task_id"` // 任务ID
}

// AudioPreprocessRequest 音频预处理请求参数
type AudioPreprocessRequest struct {
	Format      string `json:"format"`      // 目标格式 (wav)
	SampleRate  int    `json:"sample_rate"` // 采样率 (16000Hz)
	Channels    int    `json:"channels"`    // 声道数 (1=单声道)
	SampleWidth int    `json:"sample_width"`// 采样位数 (2=16bit)
}

// AudioPreprocessResponse 音频预处理响应
type AudioPreprocessResponse struct {
	Code    int    `json:"code"`    // 响应状态码
	Success bool   `json:"success"` // 请求是否成功
	Message string `json:"message"` // 响应消息
	Data    struct {
		URL      string  `json:"url"`      // 处理后的音频文件URL
		Duration int     `json:"duration"` // 音频时长（秒）
		FileSize float64 `json:"file_size"` // 文件大小（MB）
	} `json:"data"`
}

// AudioValidationError 音频验证错误
type AudioValidationError struct {
	Message string
}

func (e *AudioValidationError) Error() string {
	return e.Message
}

// ValidateAudioFile 验证音频文件
func ValidateAudioFile(filepath string) error {
	// 检查文件是否存在
	info, err := os.Stat(filepath)
	if err != nil {
		if os.IsNotExist(err) {
			return &AudioValidationError{Message: "文件不存在"}
		}
		return &AudioValidationError{Message: fmt.Sprintf("获取文件信息失败: %v", err)}
	}

	// 检查文件大小（最大5GB）
	const maxSize = 5 * 1024 * 1024 * 1024 // 5GB in bytes
	if info.Size() > maxSize {
		return &AudioValidationError{Message: fmt.Sprintf("文件大小超过限制，最大允许5GB，当前大小%.2fGB", float64(info.Size())/1024/1024/1024)}
	}

	// 检查文件扩展名
	ext := strings.ToLower(filepath[strings.LastIndex(filepath, ".")+1:])
	supportedFormats := map[string]bool{
		"wav": true,
		"mp3": true,
		"pcm": true,
		"m4a": true,
		"aac": true,
	}

	if !supportedFormats[ext] {
		return &AudioValidationError{Message: fmt.Sprintf("不支持的音频格式: %s，支持的格式: wav, mp3, pcm, m4a, aac", ext)}
	}

	return nil
}

// 生成认证签名
func (c *Client) generateSignature(timestamp string) string {
	// 签名格式：MD5(appKey + timestamp + appSecret)
	data := c.appKey + timestamp + c.appSecret
	hash := md5.Sum([]byte(data))
	return hex.EncodeToString(hash[:])
}

// RecognizeSpeech 执行语音识别
// audioData: 音频数据字节数组
// req: 识别请求参数
// 返回识别结果和可能的错误
func (c *Client) RecognizeSpeech(audioData []byte, req *ASRRequest) (*ASRResponse, error) {
	url := fmt.Sprintf("%s/asr", c.baseURL)
	
	// 创建请求体
	r := bytes.NewReader(audioData)
	
	// 创建 HTTP 请求
	request, err := http.NewRequest("POST", url, r)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}
	
	// 生成时间戳和签名
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	signature := c.generateSignature(timestamp)
	
	// 设置请求头
	request.Header.Set("X-App-Key", c.appKey)
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Signature", signature)
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("Content-Length", fmt.Sprintf("%d", len(audioData)))
	
	// 添加查询参数
	q := request.URL.Query()
	q.Add("format", req.Format)
	q.Add("sample_rate", fmt.Sprintf("%d", req.SampleRate))
	q.Add("enable_punctuation", fmt.Sprintf("%v", req.EnablePunctuation))
	q.Add("enable_normalization", fmt.Sprintf("%v", req.EnableNormalization))
	if req.TaskID != "" {
		q.Add("task_id", req.TaskID)
	}
	request.URL.RawQuery = q.Encode()
	
	// 发送请求
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %v", err)
	}
	defer response.Body.Close()
	
	// 读取响应体
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}
	
	// 解析响应
	var result ASRResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}
	
	return &result, nil
}

// RecognizeFile 从文件执行语音识别（新的方法名，更接近用户习惯）
// filepath: 音频文件路径
// taskID: 任务ID（可选）
// 返回识别结果和可能的错误
func (c *Client) RecognizeFile(filepath string, taskID string) (*ASRResponse, error) {
	// 读取文件
	audioData, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %v", err)
	}

	// 根据文件扩展名判断格式
	format := "pcm"
	if len(filepath) > 4 {
		ext := filepath[len(filepath)-4:]
		if ext == ".wav" {
			format = "wav"
		}
	}
	
	// 创建请求参数
	req := &ASRRequest{
		Format:              format,
		SampleRate:         8000,  // 默认采样率
		EnablePunctuation:  true,
		EnableNormalization: true,
		TaskID:             taskID,
	}
	
	return c.RecognizeSpeech(audioData, req)
}

// GetOSSToken 获取OSS临时访问凭证
// 返回临时凭证和可能的错误
func (c *Client) GetOSSToken() (*OSSTokenResponse, error) {
	url := fmt.Sprintf("%s/get_oss_token", c.baseURL)
	
	// 创建请求体
	reqBody := map[string]string{
		"appKey":    c.appKey,
		"appSecret": c.appSecret,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("创建请求体失败: %v", err)
	}

	// 创建 HTTP 请求
	request, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}
	
	// 设置请求头
	request.Header.Set("Content-Type", "application/json")
	
	// 发送请求
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %v", err)
	}
	defer response.Body.Close()
	
	// 读取响应体
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	// 打印调试信息
	fmt.Printf("请求URL: %s\n", url)
	fmt.Printf("状态码: %d\n", response.StatusCode)
	fmt.Printf("响应内容: %s\n", string(body))
	
	// 解析响应
	var result OSSTokenResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v, 响应内容: %s", err, string(body))
	}
	
	// 检查响应状态
	if !result.Success || result.Code != 200 {
		return nil, fmt.Errorf("获取凭证失败: %s", result.Message)
	}
	
	// 更新客户端的 OSS 凭证
	if c.ossConfig != nil {
		c.ossConfig.Credentials = result.Data
	}
	
	return &result, nil
}

// ProcessAudio 处理音频文件
func (c *Client) ProcessAudio(inputFile string) (string, error) {
	// 在当前目录下创建temp目录
	tempDir := "temp"
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("创建临时目录失败: %v", err)
	}

	// 使用相对路径创建临时文件
	outputFile := filepath.Join(tempDir, fmt.Sprintf("processed_%d.wav", time.Now().UnixNano()))

	// 打开源文件
	srcFile, err := os.OpenFile(inputFile, os.O_RDONLY, 0644)
	if err != nil {
		return "", fmt.Errorf("打开音频文件失败: %v", err)
	}
	defer srcFile.Close()

	// 获取源文件大小
	fileInfo, err := srcFile.Stat()
	if err != nil {
		return "", fmt.Errorf("获取文件信息失败: %v", err)
	}

	// 创建目标文件
	dstFile, err := os.OpenFile(outputFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return "", fmt.Errorf("创建输出文件失败: %v", err)
	}
	defer dstFile.Close()

	// 使用缓冲读写
	bufReader := bufio.NewReaderSize(srcFile, 1024*1024) // 1MB buffer
	bufWriter := bufio.NewWriterSize(dstFile, 1024*1024) // 1MB buffer
	defer bufWriter.Flush()

	// 读取WAV头部（44字节）
	header := make([]byte, 44)
	_, err = io.ReadFull(bufReader, header)
	if err != nil {
		return "", fmt.Errorf("读取WAV头部失败: %v", err)
	}

	// 检查并修改WAV头部
	if string(header[0:4]) == "RIFF" && string(header[8:12]) == "WAVE" {
		// 设置采样率为16000Hz (字节24-27)
		binary.LittleEndian.PutUint32(header[24:28], 16000)

		// 设置为单声道 (字节22-23)
		header[22] = 1
		header[23] = 0

		// 更新每秒数据字节数 (字节28-31)
		// 计算公式：采样率 * 通道数 * 位深度/8
		binary.LittleEndian.PutUint32(header[28:32], 16000*1*16/8)

		// 更新数据块大小 (字节16-19)
		binary.LittleEndian.PutUint32(header[16:20], 16)

		// 更新RIFF块大小 (字节4-7)
		binary.LittleEndian.PutUint32(header[4:8], uint32(fileInfo.Size()-8))
	}

	// 写入修改后的头部
	_, err = bufWriter.Write(header)
	if err != nil {
		return "", fmt.Errorf("写入WAV头部失败: %v", err)
	}

	// 使用较小的缓冲区复制数据，避免占用过多内存
	buf := make([]byte, 256*1024) // 256KB buffer
	_, err = io.CopyBuffer(bufWriter, bufReader, buf)
	if err != nil {
		return "", fmt.Errorf("复制音频数据失败: %v", err)
	}

	// 确保所有数据都写入磁盘
	if err := bufWriter.Flush(); err != nil {
		return "", fmt.Errorf("刷新缓冲区失败: %v", err)
	}

	if err := dstFile.Sync(); err != nil {
		return "", fmt.Errorf("同步文件失败: %v", err)
	}

	return outputFile, nil
}

// UploadFile 上传文件到 OSS
func (c *Client) UploadFile(filepath string, objectName string) (*UploadFileResponse, error) {
	// 1. 验证音频文件
	if err := ValidateAudioFile(filepath); err != nil {
		return nil, fmt.Errorf("音频文件验证失败: %v", err)
	}

	// 2. 处理音频
	processedFile, err := c.ProcessAudio(filepath)
	if err != nil {
		// 如果处理失败，尝试清理临时目录
		os.RemoveAll("temp")
		return nil, fmt.Errorf("音频处理失败: %v", err)
	}

	// 确保在函数返回时清理所有临时文件和目录
	defer func() {
		os.Remove(processedFile)
		// 尝试删除temp目录
		os.RemoveAll("temp")
	}()

	// 3. 获取 OSS 临时凭证
	tokenResp, err := c.GetOSSToken()
	if err != nil {
		return nil, fmt.Errorf("获取 OSS 凭证失败: %v", err)
	}

	// 4. 创建 OSS 客户端
	client, err := oss.New(
		c.ossConfig.Endpoint,
		tokenResp.Data.AccessKeyId,
		tokenResp.Data.AccessKeySecret,
		oss.SecurityToken(tokenResp.Data.SecurityToken),
	)
	if err != nil {
		return nil, fmt.Errorf("创建 OSS 客户端失败: %v", err)
	}

	// 5. 获取存储桶
	bucket, err := client.Bucket(c.ossConfig.BucketName)
	if err != nil {
		return nil, fmt.Errorf("获取存储桶失败: %v", err)
	}

	// 如果没有提供对象名称，则使用文件名
	if objectName == "" {
		objectName = filepath[strings.LastIndex(filepath, "/")+1:]
		if runtime.GOOS == "windows" {
			objectName = filepath[strings.LastIndex(filepath, "\\")+1:]
		}
		// 添加时间戳前缀，避免文件名冲突
		objectName = fmt.Sprintf("audio/%d_%s.wav", time.Now().Unix(), strings.TrimSuffix(filepath[strings.LastIndex(filepath, "/")+1:], path.Ext(filepath)))
	}

	// 6. 分片上传处理后的音频文件
	imur, err := bucket.InitiateMultipartUpload(objectName)
	if err != nil {
		return nil, fmt.Errorf("初始化分片上传失败: %v", err)
	}

	// 打开文件并获取信息
	file, err := os.OpenFile(processedFile, os.O_RDONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %v", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("获取文件信息失败: %v", err)
	}

	// 使用较大的分片大小，减少分片数量
	partSize := int64(20 * 1024 * 1024) // 20MB 分片大小
	numParts := (fileInfo.Size() + partSize - 1) / partSize

	// 创建上传进度条，降低刷新频率
	bar := progressbar.NewOptions64(
		fileInfo.Size(),
		progressbar.OptionSetDescription("上传文件"),
		progressbar.OptionSetWidth(15),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionShowBytes(true),
		progressbar.OptionThrottle(500*time.Millisecond), // 降低进度条更新频率到500ms
	)

	// 创建固定大小的缓冲区，避免频繁的内存分配
	buffer := make([]byte, partSize)

	// 串行上传分片，避免并发带来的资源竞争
	var parts []oss.UploadPart
	for i := int64(1); i <= numParts; i++ {
		start := (i - 1) * partSize
		size := partSize
		if i == numParts {
			if size = fileInfo.Size() - start; size < 0 {
				size = 0
			}
		}

		// 创建分片读取器
		partReader := io.NewSectionReader(file, start, size)

		// 使用固定大小的缓冲区读取数据
		n, err := io.ReadFull(partReader, buffer[:size])
		if err != nil && err != io.ErrUnexpectedEOF {
			return nil, fmt.Errorf("读取分片 %d 失败: %v", i, err)
		}

		// 创建缓冲区读取器
		reader := bytes.NewReader(buffer[:n])
		
		// 上传分片
		part, err := bucket.UploadPart(imur, reader, int64(n), int(i))
		if err != nil {
			bucket.AbortMultipartUpload(imur)
			return nil, fmt.Errorf("上传分片 %d 失败: %v", i, err)
		}

		parts = append(parts, part)
		bar.Add64(size)

		// 添加短暂延时，让出系统资源
		time.Sleep(10 * time.Millisecond)
	}

	fmt.Println() // 换行

	// 完成分片上传
	completeResult, err := bucket.CompleteMultipartUpload(imur, parts)
	if err != nil {
		return nil, fmt.Errorf("完成分片上传失败: %v", err)
	}

	// 7. 生成文件访问URL（1小时有效期）
	signedURL, err := bucket.SignURL(objectName, oss.HTTPGet, 3600)
	if err != nil {
		return nil, fmt.Errorf("生成文件访问URL失败: %v", err)
	}

	return &UploadFileResponse{
		Success: true,
		Message: fmt.Sprintf("文件上传成功，ETag: %s", completeResult.ETag),
		URL:     signedURL,
	}, nil
}

// PreprocessAudio 音频预处理
// filepath: 本地音频文件路径
// req: 预处理参数
// 返回预处理结果和可能的错误
func (c *Client) PreprocessAudio(filepath string, req *AudioPreprocessRequest) (*AudioPreprocessResponse, error) {
	// 验证音频文件
	if err := ValidateAudioFile(filepath); err != nil {
		return nil, err
	}

	// 如果没有提供请求参数，使用默认值
	if req == nil {
		req = &AudioPreprocessRequest{
			Format:      "wav",
			SampleRate:  16000,  // 16kHz
			Channels:    1,      // 单声道
			SampleWidth: 2,      // 16bit
		}
	}

	// 创建预处理请求
	preprocessURL := fmt.Sprintf("%s/preprocess_audio", c.baseURL)
	reqBody := map[string]interface{}{
		"filepath":     filepath,
		"format":       req.Format,
		"sample_rate":  req.SampleRate,
		"channels":     req.Channels,
		"sample_width": req.SampleWidth,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("创建请求体失败: %v", err)
	}

	// 创建 HTTP 请求
	request, err := http.NewRequest("POST", preprocessURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	// 生成时间戳和签名
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	signature := c.generateSignature(timestamp)

	// 设置请求头
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-App-Key", c.appKey)
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Signature", signature)

	// 发送请求
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %v", err)
	}
	defer response.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	// 打印调试信息
	fmt.Printf("预处理请求URL: %s\n", preprocessURL)
	fmt.Printf("状态码: %d\n", response.StatusCode)
	fmt.Printf("响应内容: %s\n", string(body))

	// 解析响应
	var result AudioPreprocessResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v, 响应内容: %s", err, string(body))
	}

	// 检查响应状态
	if !result.Success || result.Code != 200 {
		return nil, fmt.Errorf("预处理失败: %s", result.Message)
	}

	return &result, nil
}

// UploadPreprocessedAudio 上传预处理后的音频文件
func (c *Client) UploadPreprocessedAudio(preprocessedFilePath string, objectName string) (*UploadFileResponse, error) {
	// 获取 OSS 临时凭证
	tokenResp, err := c.GetOSSToken()
	if err != nil {
		return nil, fmt.Errorf("获取 OSS 凭证失败: %v", err)
	}

	// 创建 OSS 客户端
	client, err := oss.New(
		c.ossConfig.Endpoint,
		tokenResp.Data.AccessKeyId,
		tokenResp.Data.AccessKeySecret,
		oss.SecurityToken(tokenResp.Data.SecurityToken),
	)
	if err != nil {
		return nil, fmt.Errorf("创建 OSS 客户端失败: %v", err)
	}

	// 获取存储桶
	bucket, err := client.Bucket(c.ossConfig.BucketName)
	if err != nil {
		return nil, fmt.Errorf("获取存储桶失败: %v", err)
	}

	// 如果没有提供对象名称，则使用文件名
	if objectName == "" {
		objectName = preprocessedFilePath[strings.LastIndex(preprocessedFilePath, "/")+1:]
		if runtime.GOOS == "windows" {
			objectName = preprocessedFilePath[strings.LastIndex(preprocessedFilePath, "\\")+1:]
		}
	}

	// 上传文件
	err = bucket.PutObjectFromFile(objectName, preprocessedFilePath)
	if err != nil {
		return nil, fmt.Errorf("上传文件失败: %v", err)
	}

	// 生成文件访问URL
	signedURL, err := bucket.SignURL(objectName, oss.HTTPGet, 3600) // 1小时有效期
	if err != nil {
		return nil, fmt.Errorf("生成文件访问URL失败: %v", err)
	}

	return &UploadFileResponse{
		Success: true,
		Message: "预处理音频文件上传成功",
		URL:     signedURL,
	}, nil
}

// UploadPartInfo 分片上传信息
type UploadPartInfo struct {
	PartNumber int    // 分片号
	ETag       string // 分片的ETag
}

// MultipartUploadFile 分片上传文件到OSS
func (c *Client) MultipartUploadFile(filepath string, objectName string) (*UploadFileResponse, error) {
	// 首先获取OSS临时凭证
	tokenResp, err := c.GetOSSToken()
	if err != nil {
		return nil, fmt.Errorf("获取OSS凭证失败: %v", err)
	}

	// 创建OSS客户端
	ossClient, err := oss.New(
		c.ossConfig.Endpoint,
		tokenResp.Data.AccessKeyId,
		tokenResp.Data.AccessKeySecret,
		oss.SecurityToken(tokenResp.Data.SecurityToken),
	)
	if err != nil {
		return nil, fmt.Errorf("创建OSS客户端失败: %v", err)
	}

	// 获取存储桶
	bucket, err := ossClient.Bucket(c.ossConfig.BucketName)
	if err != nil {
		return nil, fmt.Errorf("获取存储桶失败: %v", err)
	}

	// 如果没有提供对象名称，则使用文件名
	if objectName == "" {
		objectName = filepath[strings.LastIndex(filepath, "/")+1:]
		if runtime.GOOS == "windows" {
			objectName = filepath[strings.LastIndex(filepath, "\\")+1:]
		}
	}

	// 初始化分片上传
	imur, err := bucket.InitiateMultipartUpload(objectName)
	if err != nil {
		return nil, fmt.Errorf("初始化分片上传失败: %v", err)
	}

	// 获取文件大小
	fileInfo, err := os.Stat(filepath)
	if err != nil {
		return nil, fmt.Errorf("获取文件信息失败: %v", err)
	}
	fileSize := fileInfo.Size()

	// 计算分片大小和数量
	partSize := int64(5 * 1024 * 1024) // 5MB per part
	numParts := (fileSize + partSize - 1) / partSize

	// 打开文件
	file, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %v", err)
	}
	defer file.Close()

	// 上传分片
	var parts []oss.UploadPart
	for i := int64(1); i <= numParts; i++ {
		start := (i - 1) * partSize
		size := partSize
		if i == numParts {
			if size = fileSize - start; size < 0 {
				size = 0
			}
		}

		part, err := bucket.UploadPart(imur, file, size, int(i))
		if err != nil {
			// 取消分片上传
			bucket.AbortMultipartUpload(imur)
			return nil, fmt.Errorf("上传分片 %d 失败: %v", i, err)
		}
		parts = append(parts, part)
	}

	// 完成分片上传
	completeResult, err := bucket.CompleteMultipartUpload(imur, parts)
	if err != nil {
		return nil, fmt.Errorf("完成分片上传失败: %v", err)
	}

	// 生成文件访问URL
	signedURL, err := bucket.SignURL(objectName, oss.HTTPGet, 3600) // 1小时有效期
	if err != nil {
		return nil, fmt.Errorf("生成文件访问URL失败: %v", err)
	}

	return &UploadFileResponse{
		Success: true,
		Message: fmt.Sprintf("文件上传成功，ETag: %s", completeResult.ETag),
		URL:     signedURL,
	}, nil
}

// SplitAudioFileResult 音频文件拆分结果
type SplitAudioFileResult struct {
	Success    bool     `json:"success"`    // 拆分是否成功
	Message    string   `json:"message"`    // 结果消息
	OssUrls    []string `json:"oss_urls"`   // OSS文件URL列表
	TotalParts int      `json:"total_parts"` // 总分片数
	RequestID  string   `json:"request_id"`  // 请求ID
}

// SplitAudioFile 将音频文件拆分并直接上传到OSS
func (c *Client) SplitAudioFile(filepath string, requestID string) (*SplitAudioFileResult, error) {
	// 验证音频文件
	if err := ValidateAudioFile(filepath); err != nil {
		return nil, fmt.Errorf("音频文件验证失败: %v", err)
	}

	// 获取 OSS 临时凭证
	tokenResp, err := c.GetOSSToken()
	if err != nil {
		return nil, fmt.Errorf("获取 OSS 凭证失败: %v", err)
	}

	// 打印详细的 OSS 配置信息
	fmt.Printf("\n=== OSS配置信息 ===\n")
	fmt.Printf("Endpoint: %s\n", c.ossConfig.Endpoint)
	fmt.Printf("Bucket: %s\n", c.ossConfig.BucketName)
	fmt.Printf("AccessKeyId: %s\n", tokenResp.Data.AccessKeyId)
	fmt.Printf("==================\n\n")

	// 创建 OSS 客户端
	client, err := oss.New(
		c.ossConfig.Endpoint,
		tokenResp.Data.AccessKeyId,
		tokenResp.Data.AccessKeySecret,
		oss.SecurityToken(tokenResp.Data.SecurityToken),
	)
	if err != nil {
		return nil, fmt.Errorf("创建 OSS 客户端失败: %v", err)
	}

	// 获取存储桶
	bucket, err := client.Bucket(c.ossConfig.BucketName)
	if err != nil {
		return nil, fmt.Errorf("获取存储桶失败: %v", err)
	}

	// 打开源文件
	srcFile, err := os.OpenFile(filepath, os.O_RDONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("打开源文件失败: %v", err)
	}
	defer srcFile.Close()

	// 读取WAV头部（44字节）
	header := make([]byte, 44)
	n, err := io.ReadFull(srcFile, header)
	if err != nil {
		return nil, fmt.Errorf("读取WAV头部失败: %v, 实际读取字节数: %d", err, n)
	}

	fmt.Printf("=== WAV头部信息 ===\n")
	fmt.Printf("文件标识: %s\n", string(header[0:4]))
	fmt.Printf("文件格式: %s\n", string(header[8:12]))
	fmt.Printf("==============\n\n")

	// 验证WAV格式
	if string(header[0:4]) != "RIFF" {
		return nil, fmt.Errorf("无效的WAV文件格式: 缺少RIFF标识")
	}
	if string(header[8:12]) != "WAVE" {
		return nil, fmt.Errorf("无效的WAV文件格式: 缺少WAVE标识")
	}

	// 获取音频参数
	channels := binary.LittleEndian.Uint16(header[22:24])
	sampleRate := binary.LittleEndian.Uint32(header[24:28])
	bitsPerSample := binary.LittleEndian.Uint16(header[34:36])
	
	// 获取文件大小
	fileInfo, err := srcFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("获取文件信息失败: %v", err)
	}
	totalSize := fileInfo.Size()
	dataSize := totalSize - 44 // 减去WAV头部大小

	// 计算每个分片的实际音频数据大小（100MB - WAV头部大小）
	chunkDataSize := int64(100 * 1024 * 1024) - 44
	numChunks := (dataSize + chunkDataSize - 1) / chunkDataSize

	fmt.Printf("=== 音频信息 ===\n")
	fmt.Printf("总文件大小: %.2f MB\n", float64(totalSize)/1024/1024)
	fmt.Printf("音频数据大小: %.2f MB\n", float64(dataSize)/1024/1024)
	fmt.Printf("采样率: %d Hz\n", sampleRate)
	fmt.Printf("声道数: %d\n", channels)
	fmt.Printf("采样位数: %d bit\n", bitsPerSample)
	fmt.Printf("分片大小: %.2f MB\n", float64(chunkDataSize+44)/1024/1024)
	fmt.Printf("预计分片数: %d\n", numChunks)
	fmt.Printf("==============\n\n")

	// 重置文件指针到头部
	_, err = srcFile.Seek(0, 0)
	if err != nil {
		return nil, fmt.Errorf("重置文件指针失败: %v", err)
	}

	// 创建进度条
	bar := progressbar.NewOptions64(
		dataSize,
		progressbar.OptionSetDescription("拆分并上传文件"),
		progressbar.OptionSetWidth(15),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionShowBytes(true),
		progressbar.OptionThrottle(500*time.Millisecond),
	)

	var ossUrls []string

	// 拆分并上传文件
	for i := int64(0); i < numChunks; i++ {
		// 计算当前分片的实际数据大小
		currentDataSize := chunkDataSize
		if i == numChunks-1 {
			currentDataSize = dataSize - (i * chunkDataSize)
		}

		// 创建新的WAV头部
		newHeader := make([]byte, 44)
		copy(newHeader, header)

		// 更新头部信息
		// RIFF块大小 = 文件总大小 - 8
		binary.LittleEndian.PutUint32(newHeader[4:8], uint32(currentDataSize+36))
		// 数据块大小
		binary.LittleEndian.PutUint32(newHeader[40:44], uint32(currentDataSize))

		// 创建分片缓冲区
		chunkBuffer := bytes.NewBuffer(make([]byte, 0, currentDataSize+44))
		
		// 写入WAV头部
		chunkBuffer.Write(newHeader)

		// 复制音频数据
		_, err = io.CopyN(chunkBuffer, srcFile, currentDataSize)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("复制音频数据失败: %v", err)
		}

		// 更新进度条
		bar.Add64(currentDataSize)

		// 生成OSS对象名称
		objectName := fmt.Sprintf("audio/%s/part_%d.wav", requestID, i+1)
		fmt.Printf("\n正在上传第 %d/%d 个分片: %s (%.2f MB)\n", 
			i+1, numChunks, 
			objectName, 
			float64(chunkBuffer.Len())/1024/1024,
		)

		// 上传到OSS
		err = bucket.PutObject(objectName, bytes.NewReader(chunkBuffer.Bytes()))
		if err != nil {
			return nil, fmt.Errorf("上传分片 %d 失败: %v", i+1, err)
		}

		// 验证文件是否成功上传
		exist, err := bucket.IsObjectExist(objectName)
		if err != nil {
			return nil, fmt.Errorf("验证文件上传失败: %v", err)
		}
		if !exist {
			return nil, fmt.Errorf("文件上传失败，对象不存在: %s", objectName)
		}

		// 生成文件访问URL（1小时有效期）
		signedURL, err := bucket.SignURL(objectName, oss.HTTPGet, 3600)
		if err != nil {
			return nil, fmt.Errorf("生成文件访问URL失败: %v", err)
		}
		ossUrls = append(ossUrls, signedURL)
		
		fmt.Printf("分片 %d 上传成功: %s\n", i+1, signedURL)
	}

	fmt.Printf("\n=== 上传完成 ===\n")
	fmt.Printf("总分片数: %d\n", numChunks)
	fmt.Printf("存储路径: audio/%s/\n", requestID)
	fmt.Printf("==============\n")

	return &SplitAudioFileResult{
		Success:    true,
		Message:    fmt.Sprintf("音频文件已成功拆分为 %d 个部分并上传", numChunks),
		OssUrls:    ossUrls,
		TotalParts: int(numChunks),
		RequestID:  requestID,
	}, nil
}

// min returns the smaller of two int64 values
func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}