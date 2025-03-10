# VoiceWorld Go SDK

VoiceWorld API 的 Go 语言 SDK。

## 安装

```bash
go get github.com/voiceworld/voiceworld-go-sdk
```

## 快速开始

### 初始化客户端

```go
import "github.com/voiceworld/voiceworld-go-sdk/client"

// 创建客户端实例
sdk := client.NewClient(
    "your_app_key",    // 替换为您的 AppKey
    "your_app_secret", // 替换为您的 AppSecret
)
```

### 上传音频文件

```go
// 上传音频文件
resp, err := sdk.UploadFile("path/to/your/audio.wav", "")
if err != nil {
    log.Fatalf("上传失败: %v", err)
}

fmt.Printf("文件访问URL: %s\n", resp.URL)
```

## 环境要求

- Go 1.16 或更高版本
- 支持的操作系统：Windows、Linux、macOS

## 获取帮助

如果您在使用过程中遇到任何问题，请：
1. 检查 AppKey 和 AppSecret 是否正确
2. 提交 Issue
3. 联系技术支持

## 完整示例

```go
package main

import (
    "fmt"
    "log"
    "github.com/voiceworld/voiceworld-go-sdk/client"
)

func main() {
    // 创建 SDK 客户端
    sdk := client.NewClient(
        "your_app_key",
        "your_app_secret",
    )

    // 上传音频文件
    resp, err := sdk.UploadFile("path/to/your/audio.wav", "")
    if err != nil {
        log.Fatalf("上传失败: %v", err)
    }

    // 打印处理结果
    fmt.Printf("文件访问URL: %s\n", resp.URL)
}
```

## 常见问题

1. 上传失败
   - 检查网络连接
   - 验证 API 地址是否可访问
   - 确认 AppKey 和 AppSecret 是否正确

2. 文件访问 URL 过期
   - URL 有效期为 1 小时
   - 需要重新调用相应方法获取新的 URL

3. 分片上传中断
   - 程序会自动验证每个分片的上传状态
   - 建议实现断点续传机制

## 环境要求

- Go 1.16 或更高版本
- 支持的操作系统：Windows、Linux、macOS 