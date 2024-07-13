package unpackerr

import (
	"errors"
	"sync"
	"time"

	"golift.io/starr"
	"golift.io/starr/readarr"
)

// ReadarrConfig represents the input data for a Readarr server.
type ReadarrConfig struct {
	StarrConfig
	Queue            *readarr.Queue `json:"-" toml:"-" xml:"-" yaml:"-"`
	sync.RWMutex     `json:"-" toml:"-" xml:"-" yaml:"-"`
	*readarr.Readarr `json:"-" toml:"-" xml:"-" yaml:"-"`
}

func (u *Unpackerr) validateReadarr() error {
	tmp := u.Readarr[:0]

	for idx := range u.Readarr {
		if err := u.validateApp(&u.Lidarr[idx].StarrConfig, starr.Readarr); err != nil {
			if errors.Is(err, ErrInvalidURL) {
				continue // We ignore these errors, just remove the instance from the list.
			}

			return err
		}

		u.Readarr[idx].Readarr = readarr.New(&u.Readarr[idx].Config)
		tmp = append(tmp, u.Readarr[idx])
	}

	u.Readarr = tmp

	return nil
}

func (u *Unpackerr) logReadarr() {
	if c := len(u.Readarr); c == 1 {
		u.Printf(" => Readarr Config: 1 server: "+starrLogLine,
			u.Readarr[0].URL, u.Readarr[0].APIKey != "", u.Readarr[0].Timeout,
			u.Readarr[0].ValidSSL, u.Readarr[0].Protocols, u.Readarr[0].Syncthing,
			u.Readarr[0].DeleteOrig, u.Readarr[0].DeleteDelay.Duration, u.Readarr[0].Paths)
	} else {
		u.Printf(" => Readarr Config: %d servers", c)

		for _, f := range u.Readarr {
			u.Printf(starrLogPfx+starrLogLine,
				f.URL, f.APIKey != "", f.Timeout, f.ValidSSL, f.Protocols,
				f.Syncthing, f.DeleteOrig, f.DeleteDelay.Duration, f.Paths)
		}
	}
}

// getReadarrQueue saves the Readarr Queue(s).
func (u *Unpackerr) getReadarrQueue(server *ReadarrConfig, start time.Time) {
	if server.APIKey == "" {
		u.Debugf("Readarr (%s): skipped, no API key", server.URL)
		return
	}

	queue, err := server.GetQueue(DefaultQueuePageSize, DefaultQueuePageSize)
	if err != nil {
		u.saveQueueMetrics(0, start, starr.Readarr, server.URL, err)
		return
	}

	// Only update if there was not an error fetching.
	server.Queue = queue
	u.saveQueueMetrics(server.Queue.TotalRecords, start, starr.Readarr, server.URL, nil)

	if !u.Activity || queue.TotalRecords > 0 {
		u.Printf("[Readarr] Updated (%s): %d Items Queued, %d Retrieved", server.URL, queue.TotalRecords, len(queue.Records))
	}
}

// checkReadarQueue saves completed Readarr-queued downloads to u.Map.
func (u *Unpackerr) checkReadarrQueue(now time.Time) {
	for _, server := range u.Readarr {
		if server.Queue == nil {
			continue
		}

		for _, q := range server.Queue.Records {
			switch x, ok := u.Map[q.Title]; {
			case ok && x.Status == EXTRACTED && u.isComplete(q.Status, q.Protocol, server.Protocols):
				u.Debugf("%s (%s): Item Waiting for Import (%s): %v", starr.Readarr, server.URL, q.Protocol, q.Title)
			case !ok && u.isComplete(q.Status, q.Protocol, server.Protocols):
				u.Map[q.Title] = &Extract{
					App:         starr.Readarr,
					URL:         server.URL,
					Updated:     now,
					Status:      WAITING,
					DeleteOrig:  server.DeleteOrig,
					DeleteDelay: server.DeleteDelay.Duration,
					Syncthing:   server.Syncthing,
					Path:        u.getDownloadPath(q.OutputPath, starr.Readarr, q.Title, server.Paths),
					IDs: map[string]interface{}{
						"title":      q.Title,
						"authorId":   q.AuthorID,
						"bookId":     q.BookID,
						"downloadId": q.DownloadID,
						"reason":     buildStatusReason(q.Status, q.StatusMessages),
					},
				}

				fallthrough
			default:
				u.Debugf("%s: (%s): %s (%s:%d%%): %v",
					starr.Readarr, server.URL, q.Status, q.Protocol, percent(q.Sizeleft, q.Size), q.Title)
			}
		}
	}
}

// checks if the application currently has an item in its queue.
func (u *Unpackerr) haveReadarrQitem(name string) bool {
	for _, server := range u.Readarr {
		if server.Queue == nil {
			continue
		}

		for _, q := range server.Queue.Records {
			if q.Title == name {
				return true
			}
		}
	}

	return false
}
