package stats

import (
	"encoding/csv"
	"strconv"
)

func FlushStats(writer *csv.Writer, owner, repo string) {
	fields := make([]string, len(AllStats)+2)
	fields[0] = owner
	fields[1] = repo
	for idx, stat := range AllStats {
		fields[idx+2] = strconv.Itoa(GetCount(stat))
	}
	writer.Write(fields)
	Clear()
}