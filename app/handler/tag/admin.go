package tag

import (
	"github.com/gin-gonic/gin"
)

// AdminGetTagList 获取标签列表
func (t *tagHandler) AdminGetTagList(c *gin.Context) {}

// AdminGetArticleListByTag 获取标签下的文章列表
func (t *tagHandler) AdminGetArticleListByTag(c *gin.Context) {}

// AdminUpdateTag 更新标签
func (t *tagHandler) AdminUpdateTag(c *gin.Context) {}
