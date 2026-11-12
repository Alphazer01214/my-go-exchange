package api

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

func (s *Server) health(c *gin.Context) {
	writeData(c, 200, gin.H{"status": "ok"})
}

func (s *Server) listInstruments(c *gin.Context) {
	instruments := s.market.Instruments()
	views := make([]MarketView, 0, len(instruments))
	for _, instrument := range instruments {
		views = append(views, marketView(instrument))
	}
	writeData(c, 200, views)
}

func (s *Server) getBook(c *gin.Context) {
	symbol := c.Query("symbol")
	if symbol == "" {
		writeError(c, 400, "invalid_request", "symbol is required")
		return
	}

	depth := 20
	if rawDepth := c.Query("depth"); rawDepth != "" {
		parsed, err := strconv.Atoi(rawDepth)
		if err != nil || parsed <= 0 || parsed > 1000 {
			writeError(c, 400, "invalid_request", "depth must be between 1 and 1000")
			return
		}
		depth = parsed
	}

	book, err := s.market.Book(c.Request.Context(), symbol)
	if err != nil {
		writeMarketError(c, err)
		return
	}
	if len(book.Bids) > depth {
		book.Bids = book.Bids[:depth]
	}
	if len(book.Asks) > depth {
		book.Asks = book.Asks[:depth]
	}
	writeData(c, 200, book)
}
