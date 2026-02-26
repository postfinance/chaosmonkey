package main

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"time"

	"github.com/jfyne/live"

	"github.com/postfinance/chaosmonkey/pkg/profile"
)

// RegisterDashboard registers the live dashboard on the given mux.
func (m *Monkey) RegisterDashboard(mux *http.ServeMux, ctx context.Context) {
	mux.Handle("/", m.newDashboardHandler(ctx))
	mux.Handle("/live.js", live.Javascript{})
	mux.Handle("/auto.js.map", live.JavascriptMap{})
}

// dashboardModel is the live view model.
type dashboardModel struct {
	TotalEvicted  int
	TotalErrors   int
	Uptime        string
	Timezone      string
	Now           time.Time
	Suspended     bool
	SuspendSince  string
	SuspendReason string
	Upcoming      []*podEntry
	Recent        []*podEntry
	Profiles      []profileView
	SuspendEvents []suspendEvent

	// Dead man's switch
	DMSEnabled    bool
	DMSExpired    bool
	DMSAutoResume bool
	DMSLeaseValue string
	DMSLeaseTitle string
	DMSLeaseOK    bool
}

func (m *Monkey) newDashboardModel() *dashboardModel {
	now := time.Now().In(m.location)
	upcoming, recent := m.sched.snapshot()
	if len(upcoming) > 10 {
		upcoming = upcoming[:10]
	}
	if len(recent) > 10 {
		recent = recent[:10]
	}

	suspended, reason, since := m.suspend.status()
	suspendReason := "-"
	suspendSince := "-"
	if suspended {
		suspendReason = reason
		suspendSince = since.In(m.location).Format("2006-01-02 15:04:05")
	}

	model := &dashboardModel{
		TotalEvicted:  int(m.totalKilled.Load()),
		TotalErrors:   int(m.totalErrors.Load()),
		Uptime:        now.Sub(m.startTime).Truncate(time.Second).String(),
		Timezone:      m.location.String(),
		Now:           now,
		Suspended:     suspended,
		SuspendSince:  suspendSince,
		SuspendReason: suspendReason,
		Upcoming:      upcoming,
		Recent:        recent,
		Profiles:      m.newProfilesModel(),
		SuspendEvents: m.eventLog.snapshot(),
	}

	if m.dms != nil {
		enabled, expired, lastRenew, expiresAt, _ := m.dms.Status()
		model.DMSEnabled = enabled
		model.DMSExpired = expired
		model.DMSAutoResume = m.dms.AutoResume()
		if !expiresAt.IsZero() {
			remaining := expiresAt.Sub(now).Truncate(time.Second)
			model.DMSLeaseValue = remaining.String()
			model.DMSLeaseTitle = fmt.Sprintf("Expires: %s · Renewed: %s",
				expiresAt.In(m.location).Format("15:04:05"),
				lastRenew.In(m.location).Format("15:04:05"))
			model.DMSLeaseOK = remaining > 0
		} else {
			model.DMSLeaseValue = "no lease"
			model.DMSLeaseTitle = "Lease not found"
		}
	}

	return model
}

const liveTickEvent = "tick"

func (m *Monkey) newDashboardHandler(ctx context.Context) *live.Engine {
	h := live.NewHandler(live.WithTemplateRenderer(dashboardTmpl))

	h.MountHandler = func(_ context.Context, s *live.Socket) (any, error) {
		model := m.newDashboardModel()
		if s.Connected() {
			go func() {
				time.Sleep(1 * time.Second)
				s.Self(ctx, liveTickEvent, nil)
			}()
		}
		return model, nil
	}

	h.HandleSelf(liveTickEvent, func(_ context.Context, s *live.Socket, _ any) (any, error) {
		go func() {
			time.Sleep(1 * time.Second)
			s.Self(ctx, liveTickEvent, nil)
		}()
		return m.newDashboardModel(), nil
	})

	h.HandleEvent("toggle-suspend", func(_ context.Context, _ *live.Socket, _ live.Params) (any, error) {
		if m.suspend.isSuspended() {
			m.Resume("manual (dashboard)")
		} else {
			m.Suspend("manual (dashboard)")
		}
		return m.newDashboardModel(), nil
	})

	return live.NewHttpHandler(ctx, h)
}

type profileView struct {
	Name          string
	KillMode      string
	MinAge        string
	MaxAge        string
	ExcludedTimes []string
	ExcludedDates []string
	ExcludedDays  []string
}

func (m *Monkey) newProfilesModel() []profileView {
	names := m.profileNames()
	sort.Strings(names)

	var views []profileView
	for _, name := range names {
		p := m.profiles[name]
		km := string(p.KillMode)
		if km == "" {
			km = string(profile.KillModeEvict)
		}
		v := profileView{
			Name:     name,
			KillMode: km,
			MinAge:   p.MinAge.String(),
			MaxAge:   p.MaxAge.String(),
		}
		if p.MinAge == 0 {
			v.MinAge = "-"
		}
		for _, tr := range p.ExcludedTimes {
			v.ExcludedTimes = append(v.ExcludedTimes, tr.String())
		}
		for _, d := range p.ExcludedDates {
			v.ExcludedDates = append(v.ExcludedDates, d.Format("2006-01-02"))
		}
		for _, d := range p.ExcludedDays {
			v.ExcludedDays = append(v.ExcludedDays, d.String())
		}
		views = append(views, v)
	}
	return views
}

func timeAgo(t time.Time, now time.Time) string {
	d := now.Sub(t).Truncate(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh%dm ago", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd%dh ago", int(d.Hours())/24, int(d.Hours())%24)
}

func countdown(t time.Time, now time.Time) string {
	d := t.Sub(now).Truncate(time.Second)
	if d < 0 {
		return "overdue"
	}
	if d < time.Minute {
		return fmt.Sprintf("in %ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("in %dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("in %dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("in %dd%dh", int(d.Hours())/24, int(d.Hours())%24)
}

var cssStyle = `
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: system-ui, -apple-system, sans-serif; background: #f5f5f5; color: #333; padding: 2rem; border-top: 3px solid #e74c3c; transition: border-top .3s; }
  body.live-connected { border-top: 3px solid #2ecc71; }
  h1 { margin-bottom: 1.5rem; font-size: 1.5rem; }
  .cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: .75rem; margin-bottom: 2rem; }
  .card { background: #fff; border-radius: 8px; padding: .85rem 1rem; box-shadow: 0 1px 3px rgba(0,0,0,.1); }
  .card .label { font-size: .65rem; text-transform: uppercase; color: #888; letter-spacing: .05em; }
  .card .value { font-size: 1.3rem; font-weight: 600; margin-top: .2rem; font-variant-numeric: tabular-nums; }
  .card-wide { grid-column: span 2; }
  .card-state { display: flex; align-items: center; justify-content: space-between; gap: .5rem; }
  .card-state .left { display: flex; align-items: center; gap: .4rem; }
  .card-state .dot { width: 10px; height: 10px; border-radius: 50%; flex-shrink: 0; }
  .card-state .dot.running { background: #2ecc71; }
  .card-state .dot.suspended { background: #e74c3c; }
  .card-state .info { font-size: .7rem; color: #888; }
  .toggle { padding: .3rem .75rem; border: none; border-radius: 5px; font-size: .7rem; font-weight: 600; cursor: pointer; white-space: nowrap; background: #ddd; color: #333; }
  .toggle:hover { background: #ccc; }
  .val-ok { color: #2ecc71; }
  .val-bad { color: #e74c3c; }
  h2 { font-size: 1rem; margin: 1.5rem 0 .5rem; }
  table { width: 100%; border-collapse: collapse; background: #fff; border-radius: 8px; overflow: hidden; box-shadow: 0 1px 3px rgba(0,0,0,.1); margin-bottom: 1rem; }
  th, td { text-align: left; padding: .4rem .6rem; border-bottom: 1px solid #eee; font-size: .75rem; }
  th { background: #fafafa; font-weight: 600; color: #666; }
  tr:last-child td { border-bottom: none; }
  .empty { padding: 1rem; text-align: center; color: #999; font-size: .8rem; }
  .tag { display: inline-block; background: #eee; border-radius: 4px; padding: .1rem .3rem; font-size: .65rem; margin-right: .15rem; }
  .section { margin-bottom: 1.5rem; }
  .note { font-size: .65rem; color: #999; margin-top: -.5rem; margin-bottom: .75rem; }
`

var dashboardTmpl = template.Must(template.New("root").Funcs(template.FuncMap{
	"timeAgo":   timeAgo,
	"countdown": countdown,
	"fmtTime": func(t time.Time, now time.Time) string {
		return t.In(now.Location()).Format("2006-01-02 15:04:05")
	},
}).Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Chaos Monkey</title>
<style>` + cssStyle + `</style>
</head>
<body>
<h1>Chaos Monkey</h1>

<div class="cards">
  <div class="card card-wide">
    <div class="label">State</div>
    <div class="card-state">
      <div class="left">
        <div class="dot {{if .Assigns.Suspended}}suspended{{else}}running{{end}}"></div>
        <div class="value" style="margin:0">{{if .Assigns.Suspended}}Suspended{{else}}Running{{end}}</div>
      </div>
      <button class="toggle" live-click="toggle-suspend">{{if .Assigns.Suspended}}Resume{{else}}Suspend{{end}}</button>
    </div>
  </div>
  <div class="card card-wide"><div class="label">Suspended Since</div><div class="value">{{.Assigns.SuspendSince}}</div></div>
  <div class="card"><div class="label">Suspend Reason</div><div class="value">{{.Assigns.SuspendReason}}</div></div>
  <div class="card"><div class="label">Evicted</div><div class="value">{{.Assigns.TotalEvicted}}</div></div>
  <div class="card"><div class="label">Errors</div><div class="value">{{.Assigns.TotalErrors}}</div></div>
  <div class="card"><div class="label">Profiles</div><div class="value">{{len .Assigns.Profiles}}</div></div>
  <div class="card"><div class="label">Uptime</div><div class="value">{{.Assigns.Uptime}}</div></div>
  <div class="card"><div class="label">Timezone</div><div class="value">{{.Assigns.Timezone}}</div></div>
  {{if .Assigns.DMSEnabled}}<div class="card"><div class="label">DMS Lease</div><div class="value {{if .Assigns.DMSLeaseOK}}val-ok{{else}}val-bad{{end}}" title="{{.Assigns.DMSLeaseTitle}}">{{.Assigns.DMSLeaseValue}}</div></div>
  <div class="card"><div class="label">DMS Auto-Resume</div><div class="value">{{if .Assigns.DMSAutoResume}}On{{else}}Off{{end}}</div></div>{{end}}
</div>

<div class="section">
<h2>Profiles</h2>
<table>
<tr><th>Name</th><th>Kill Mode</th><th>Min Age</th><th>Max Age</th><th>Excluded Times</th><th>Excluded Days</th><th>Excluded Dates</th></tr>
{{range .Assigns.Profiles}}<tr>
  <td><strong>{{.Name}}</strong></td>
  <td>{{.KillMode}}</td>
  <td>{{.MinAge}}</td>
  <td>{{.MaxAge}}</td>
  <td>{{range .ExcludedTimes}}<span class="tag">{{.}}</span>{{else}}-{{end}}</td>
  <td>{{range .ExcludedDays}}<span class="tag">{{.}}</span>{{else}}-{{end}}</td>
  <td>{{range .ExcludedDates}}<span class="tag">{{.}}</span>{{else}}-{{end}}</td>
</tr>{{end}}
</table>
</div>

{{if .Assigns.SuspendEvents}}
<div class="section">
<h2>Suspend / Resume Events</h2>
<table>
<tr><th>Time</th><th>Action</th><th>Reason</th></tr>
{{range .Assigns.SuspendEvents}}<tr><td>{{fmtTime .Time $.Assigns.Now}}</td><td>{{.Action}}</td><td>{{if .Reason}}{{.Reason}}{{else}}-{{end}}</td></tr>
{{end}}
</table>
</div>
{{end}}

<div class="section">
<h2>Recently Killed</h2>
{{if .Assigns.Recent}}
<table>
<tr><th>Namespace</th><th>Pod</th><th>Profile</th><th>Result</th><th>When</th></tr>
{{range .Assigns.Recent}}<tr><td>{{.Namespace}}</td><td>{{.Name}}</td><td>{{.Profile}}</td><td>{{.Result}}</td><td>{{timeAgo .KilledAt $.Assigns.Now}}</td></tr>
{{end}}
</table>
{{else}}<div class="empty">No pods killed yet.</div>{{end}}
</div>

<div class="section">
<h2>Upcoming Kills</h2>
{{if .Assigns.Upcoming}}
<table>
<tr><th>Namespace</th><th>Pod</th><th>Profile</th><th>Mode</th><th>Kill Time</th><th>Countdown</th></tr>
{{range .Assigns.Upcoming}}<tr><td>{{.Namespace}}</td><td>{{.Name}}</td><td>{{.Profile}}</td><td>{{.KillMode}}</td><td>{{fmtTime .KillTime $.Assigns.Now}}{{if not .Deterministic}}*{{end}}</td><td>{{countdown .KillTime $.Assigns.Now}}</td></tr>
{{end}}
</table>
<div class="note">* non-deterministic due to resume/startup</div>
{{else}}<div class="empty">No upcoming kills.</div>{{end}}
</div>

<script src="/live.js"></script>
</body>
</html>
`))
