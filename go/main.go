package main

import (
	"fmt"
	"log"
	"time"
	"voiceworld/voiceworld/client"
)

func main() {
	// SDK配置
	appKey := "2mZJHE9gi51JIHHv"
	appSecret := "7CF0823C67F050CDED5543C021DEC1B7"
	audioFile := "D:\\code\\voiceworld\\voiceworld-go-sdk\\test_audio.wav"  // 要处理的音频文件路径

	// 创建 SDK 客户端
	sdk := client.NewClient(
		appKey,
		appSecret,
		&client.ClientConfig{
			BaseURL: "http://localhost:8000",
			OSSConfig: &client.OSSConfig{
				Endpoint:   "https://oss-cn-shanghai.aliyuncs.com",
				BucketName: "voiceworld",
			},
		},
	)

	// 生成请求ID（使用时间戳）
	requestID := fmt.Sprintf("%d", time.Now().Unix())

	fmt.Printf("开始处理音频文件: %s\n", audioFile)
	fmt.Printf("请求ID: %s\n\n", requestID)

	// 拆分并上传音频文件
	result, err := sdk.SplitAudioFile(audioFile, requestID)
	if err != nil {
		log.Fatalf("处理失败: %v", err)
	}

	// 打印处理结果
	fmt.Printf("\n=== 处理结果 ===\n")
	fmt.Printf("状态: %v\n", result.Success)
	fmt.Printf("消息: %s\n", result.Message)
	fmt.Printf("总分片数: %d\n", result.TotalParts)
	fmt.Printf("请求ID: %s\n", result.RequestID)
	fmt.Println("\n文件访问URL列表:")
	for i, url := range result.OssUrls {
		fmt.Printf("%d. %s\n", i+1, url)
	}
	fmt.Println("===============")
}

