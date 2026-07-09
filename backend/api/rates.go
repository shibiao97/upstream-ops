package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func registerRates(g *gin.RouterGroup, d *Deps) {
	g.GET("/rate-changes", func(c *gin.Context) {
		var channelID uint
		if s := c.Query("channel_id"); s != "" {
			id, err := strconv.ParseUint(s, 10, 64)
			if err != nil {
				fail(c, http.StatusBadRequest, err)
				return
			}
			channelID = uint(id)
		}
		page, pageSize, err := parsePageQuery(c)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ids, err := visibleChannelIDs(c, d)
		if err != nil {
			fail(c, http.StatusUnauthorized, err)
			return
		}
		list, total, err := d.Rates.ListChangesPageVisible(channelID, ids, page, pageSize)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		pages := 1
		if total > 0 {
			pages = int((total + int64(pageSize) - 1) / int64(pageSize))
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{
			"items":     list,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
			"pages":     pages,
		}})
	})
}
