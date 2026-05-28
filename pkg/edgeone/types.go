package edgeone

import (
	"context"

	teo "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/teo/v20220901"
)

// purgeAPI 抽象 EdgeOne SDK 中本包用到的方法子集。
//
// 引入这层接口的唯一目的是单测里能注入 fake 实现，避免真的访问腾讯云。
// 生产代码中由 *teo.Client 自动满足该接口，零运行时开销。
type purgeAPI interface {
	CreatePurgeTaskWithContext(ctx context.Context, req *teo.CreatePurgeTaskRequest) (
		*teo.CreatePurgeTaskResponse, error)
}
