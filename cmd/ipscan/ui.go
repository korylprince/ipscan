package main

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var headers = []string{"IP Address", "Hostname", "Pings", "Avg", "Min", "Max", "Error"}

func update(app *tview.Application, t *tview.Table, footer *tview.TextView, list *List) {
	for {
		app.QueueUpdateDraw(func() {
			for y, host := range list.List() {
				host.mu.RLock()
				if host.IP == nil {
					t.SetCell(y+1, 0, tview.NewTableCell("..."))
				} else {
					t.SetCell(y+1, 0, tview.NewTableCell(host.IP.String()))
				}

				if host.resolving {
					t.SetCell(y+1, 1, tview.NewTableCell("..."))
				} else if dErr, ok := host.ResolveError.(*net.DNSError); ok && dErr.Err == "Name or service not known" {
					t.SetCell(y+1, 1, tview.NewTableCell(""))
				} else if host.ResolveError != nil {
					t.SetCell(y+1, 1, tview.NewTableCell(host.ResolveError.Error()).SetBackgroundColor(tcell.ColorRed))
				} else {
					t.SetCell(y+1, 1, tview.NewTableCell(host.Hostname))
				}

				if host.Sent == 0 {
					t.SetCell(y+1, 2, tview.NewTableCell("..."))
					t.SetCell(y+1, 3, tview.NewTableCell("..."))
					t.SetCell(y+1, 4, tview.NewTableCell("..."))
					t.SetCell(y+1, 5, tview.NewTableCell("..."))
				} else {
					t.SetCell(y+1, 2, tview.NewTableCell(fmt.Sprintf("%d/%d (%.1f%%)",
						host.Recv, host.Sent, float64(host.Recv)/float64(host.Sent)*100,
					)))
					t.SetCell(y+1, 3, tview.NewTableCell(fmt.Sprintf("%.3fms", float64(host.Avg.Microseconds())/1000)))
					t.SetCell(y+1, 4, tview.NewTableCell(fmt.Sprintf("%.3fms", float64(host.Min.Microseconds())/1000)))
					t.SetCell(y+1, 5, tview.NewTableCell(fmt.Sprintf("%.3fms", float64(host.Max.Microseconds())/1000)))
				}

				if host.PingError != nil {
					t.SetCell(y+1, 6, tview.NewTableCell(host.PingError.Error()))
				} else {
					t.SetCell(y+1, 6, tview.NewTableCell(""))
				}

				if host.Last != nil && host.Last.RecvTime == nil {
					for i := range headers {
						t.GetCell(y+1, i).SetBackgroundColor(tcell.ColorRed)
					}
				}
				host.mu.RUnlock()
			}
		})

		footer.SetText(fmt.Sprintf("Total Sent: %d", list.Stat()))
		if err := list.GetError(); err != nil {
			footer.SetText(fmt.Sprintf("%s, Error: %v", footer.GetText(true), err))
		}
		time.Sleep(time.Second)
	}
}

func ui(listeners []*net.IPAddr, list *List) {
	app := tview.NewApplication()
	header := tview.NewTextView()
	footer := tview.NewTextView()
	table := tview.NewTable().
		// SetBorders(true).
		SetFixed(1, 2).
		SetSelectable(true, false)

	for i, h := range headers {
		table.SetCell(0, i, tview.NewTableCell(h).SetTextColor(tcell.ColorYellowGreen).
			SetSelectable(false).
			SetExpansion(1))
	}

	go update(app, table, footer, list)

	ipStr := make([]string, 0, len(listeners))
	for _, ip := range listeners {
		ipStr = append(ipStr, ip.String())
	}

	header.SetText(fmt.Sprintf("ipscan\tListening on: %s", strings.Join(ipStr, ", ")))

	footer.SetText("footer").
		SetChangedFunc(func() {
			app.Draw()
		})

	grid := tview.NewGrid().
		SetRows(1, 0, 1).
		SetBorders(true).
		AddItem(header, 0, 0, 1, 1, 0, 0, false).
		AddItem(table, 1, 0, 1, 1, 0, 0, false).
		AddItem(footer, 2, 0, 1, 1, 0, 0, false)

	grid.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape || event.Rune() == 'q' {
			app.Stop()
		}
		return event
	})

	if err := app.SetRoot(grid, true).SetFocus(table).Run(); err != nil {
		fmt.Println("Unable to start ui:", err)
		os.Exit(1)
	}
}
