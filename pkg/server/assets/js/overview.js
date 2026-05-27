// Overview page controller
class OverviewPage {
    constructor() {
        this.refs = {
            kpiActive: document.getElementById('kpiActive'),
            kpiActiveSub: document.getElementById('kpiActiveSub'),
            kpiQueue: document.getElementById('kpiQueue'),
            kpiQueueSub: document.getElementById('kpiQueueSub'),
            kpiThroughput: document.getElementById('kpiThroughput'),
            kpiThroughputSub: document.getElementById('kpiThroughputSub'),
            kpiErrors: document.getElementById('kpiErrors'),
            kpiErrorsSub: document.getElementById('kpiErrorsSub'),
            providerGrid: document.getElementById('providerGrid'),
            providersUpdated: document.getElementById('providersUpdated'),
            activeStrip: document.getElementById('activeStrip'),
            bwCapLabel: document.getElementById('bwCapLabel'),
            chartContainer: document.getElementById('overviewThroughputChart'),
        };
        this.buffer = window.pollingBuffer('overview-throughput', 60);
        this.chart = null;
        this.refreshIntervalMs = 5000;
        this.timer = null;
        this.init();
    }

    async init() {
        this.initChart();
        await this.refresh();
        this.timer = setInterval(() => this.refresh(), this.refreshIntervalMs);
        window.addEventListener('pageHidden', () => clearInterval(this.timer));
        window.addEventListener('pageVisible', () => {
            this.refresh();
            this.timer = setInterval(() => this.refresh(), this.refreshIntervalMs);
        });
    }

    initChart() {
        if (!window.ApexCharts || !this.refs.chartContainer) return;
        this.chart = new ApexCharts(this.refs.chartContainer, {
            chart: {
                type: 'area',
                height: 220,
                animations: { enabled: false },
                toolbar: { show: false },
                zoom: { enabled: false },
                background: 'transparent',
            },
            theme: { mode: document.documentElement.getAttribute('data-theme') === 'light' ? 'light' : 'dark' },
            stroke: { curve: 'smooth', width: 2 },
            dataLabels: { enabled: false },
            series: [{ name: 'Throughput', data: [] }],
            xaxis: {
                type: 'datetime',
                labels: { datetimeUTC: false, style: { fontSize: '11px' } },
            },
            yaxis: {
                labels: {
                    style: { fontSize: '11px' },
                    formatter: (v) => window.decypharrUtils.formatBytes(v) + '/s',
                },
            },
            tooltip: {
                x: { format: 'HH:mm:ss' },
                y: { formatter: (v) => window.decypharrUtils.formatBytes(v) + '/s' },
            },
            colors: ['#4f8cff'],
            grid: { borderColor: 'rgba(127,127,127,0.15)' },
        });
        this.chart.render();
        this._resizeHandler = () => {
            if (this.chart) this.chart.resize();
        };
        window.addEventListener('resize', this._resizeHandler);
    }

    async refresh() {
        try {
            const [debridRes, summaryRes, torrentsRes, configRes] = await Promise.all([
                window.decypharrUtils.fetcher('/api/debrid/status'),
                window.decypharrUtils.fetcher('/api/queue/summary'),
                window.decypharrUtils.fetcher('/api/torrents?limit=50&state=downloading&sort_by=speed&sort_order=desc'),
                window.decypharrUtils.fetcher('/api/bandwidth').catch(() => null),
            ]);

            const debrid = debridRes.ok ? await debridRes.json() : { providers: [] };
            const summary = summaryRes.ok ? await summaryRes.json() : { total: 0, by_state: {} };
            const torrents = torrentsRes.ok ? (await torrentsRes.json()) : { torrents: [] };
            const bandwidth = configRes && configRes.ok ? await configRes.json() : null;

            this.renderKPIs(summary, torrents);
            this.renderProviders(debrid.providers || []);
            this.renderActive(torrents.torrents || []);
            this.updateThroughputChart(torrents.torrents || []);
            this.renderBwCap(bandwidth);
        } catch (err) {
            console.error('Overview refresh failed:', err);
        }
    }

    renderKPIs(summary, torrents) {
        const list = torrents.torrents || [];
        const active = list.length;
        const totalSpeed = list.reduce((s, t) => s + (t.dlspeed || t.speed || 0), 0);
        this.refs.kpiActive.textContent = active.toString();
        this.refs.kpiQueue.textContent = (summary.total || 0).toString();
        this.refs.kpiThroughput.textContent = totalSpeed > 0
            ? window.decypharrUtils.formatBytes(totalSpeed) + '/s'
            : 'idle';
        const errors = (summary.errors || []).length || (summary.by_state && summary.by_state.error) || 0;
        this.refs.kpiErrors.textContent = errors.toString();
        this.refs.kpiErrorsSub.textContent = errors > 0 ? 'needs attention' : 'all clear';
    }

    renderProviders(providers) {
        if (!providers.length) {
            this.refs.providerGrid.innerHTML = `
                <div class="provider-card opacity-60">
                    <div class="kpi-label">No providers configured</div>
                    <div class="kpi-sub">Add a debrid provider in Settings to get started.</div>
                </div>`;
            return;
        }
        this.refs.providerGrid.innerHTML = providers.map(p => {
            const slug = window.providerSlug(p.type || p.name);
            const color = window.providerColor(p.type || p.name);
            const status = p.status || 'unknown';
            const statusClass = status === 'active' ? 'badge-success' : status === 'error' ? 'badge-error' : 'badge-ghost';
            const points = (p.points !== undefined && p.points !== null) ? p.points : null;
            const expiry = p.expiry ? new Date(p.expiry).toLocaleDateString() : '—';
            return `
                <div class="provider-card" data-prov="${slug}" style="--prov-color:${color}">
                    <div class="provider-card-header">
                        <div class="provider-card-title">
                            <span class="prov-chip-dot"></span>
                            ${window.decypharrUtils.escapeHtml(p.name)}
                        </div>
                        <span class="badge ${statusClass} badge-sm capitalize">${status}</span>
                    </div>
                    <div class="provider-card-meta">
                        <div>
                            <div class="meta-label">Type</div>
                            <div class="meta-value capitalize">${window.decypharrUtils.escapeHtml(p.type || '—')}</div>
                        </div>
                        <div>
                            <div class="meta-label">Premium</div>
                            <div class="meta-value">${p.premium ? 'Yes' : 'No'}</div>
                        </div>
                        <div>
                            <div class="meta-label">Points</div>
                            <div class="meta-value">${points !== null ? points : '—'}</div>
                        </div>
                        <div>
                            <div class="meta-label">Expires</div>
                            <div class="meta-value">${expiry}</div>
                        </div>
                    </div>
                    ${p.error ? `<div class="alert alert-error alert-sm text-xs p-2">${window.decypharrUtils.escapeHtml(p.error)}</div>` : ''}
                    <div class="flex justify-end">
                        <button class="btn btn-ghost btn-xs" data-test-provider="${window.decypharrUtils.escapeHtml(p.name)}">
                            <i class="bi bi-speedometer"></i> Test
                        </button>
                    </div>
                </div>
            `;
        }).join('');
        this.refs.providersUpdated.textContent = 'updated ' + new Date().toLocaleTimeString();

        this.refs.providerGrid.querySelectorAll('[data-test-provider]').forEach(btn => {
            btn.addEventListener('click', () => this.testProvider(btn.dataset.testProvider));
        });
    }

    async testProvider(name) {
        try {
            window.decypharrUtils.createToast(`Testing ${name}…`, 'info', 3000);
            const res = await window.decypharrUtils.fetcher(`/api/debrid/providers/${encodeURIComponent(name)}/test`, {
                method: 'POST',
            });
            const data = await res.json().catch(() => ({}));
            if (res.ok) {
                window.decypharrUtils.createToast(`${name}: ${data.message || 'OK'}`, 'success');
            } else {
                window.decypharrUtils.createToast(`${name}: ${data.error || 'Failed'}`, 'error');
            }
        } catch (err) {
            window.decypharrUtils.createToast(`Test failed: ${err.message}`, 'error');
        }
    }

    renderActive(list) {
        const top = list.slice(0, 8);
        if (!top.length) {
            this.refs.activeStrip.innerHTML = `<div class="opacity-60 text-sm p-3">No active downloads.</div>`;
            return;
        }
        this.refs.activeStrip.innerHTML = top.map(t => {
            const provider = t.debrid || t.active_provider || '';
            const slug = window.providerSlug(provider);
            const color = window.providerColor(provider);
            const speed = t.dlspeed || t.speed || 0;
            const pct = Math.round((t.progress || 0) * 100);
            const phaseHint = {
                debrid_fetching: 'Provider cache',
                downloading: t.action === 'download' ? 'Local pull' : 'Post-process',
                importing: 'Linking',
            }[t.phase] || '';
            const meta = [
                provider ? window.decypharrUtils.escapeHtml(provider) : '',
                phaseHint,
                `${pct}%`,
            ].filter(Boolean).join(' · ');
            return `
                <div class="active-strip-row" data-prov="${slug}" style="--prov-color:${color}">
                    <div>
                        <div class="active-strip-name">${window.decypharrUtils.escapeHtml(t.name)}</div>
                        <div class="text-xs opacity-60">${meta}</div>
                    </div>
                    <div class="text-xs font-mono">${speed > 0 ? window.decypharrUtils.formatBytes(speed) + '/s' : '—'}</div>
                    <progress class="progress progress-info w-20" value="${pct}" max="100"></progress>
                </div>
            `;
        }).join('');
    }

    updateThroughputChart(torrents) {
        const now = Date.now();
        const total = torrents.reduce((s, t) => s + (t.dlspeed || t.speed || 0), 0);
        this.buffer.push([now, total]);
        if (this.chart) {
            this.chart.updateSeries([{ name: 'Throughput', data: this.buffer.values() }]);
        }
    }

    renderBwCap(bw) {
        if (!this.refs.bwCapLabel) return;
        if (!bw || (!bw.global_bytes_per_sec && (!bw.per_provider_bytes_per_sec || !Object.keys(bw.per_provider_bytes_per_sec).length))) {
            this.refs.bwCapLabel.textContent = 'cap: unlimited';
            return;
        }
        const parts = [];
        if (bw.global_bytes_per_sec) {
            parts.push(`global ${window.decypharrUtils.formatBytes(bw.global_bytes_per_sec)}/s`);
        }
        const perProv = bw.per_provider_bytes_per_sec || {};
        Object.keys(perProv).forEach(k => {
            parts.push(`${k} ${window.decypharrUtils.formatBytes(perProv[k])}/s`);
        });
        this.refs.bwCapLabel.textContent = parts.length ? parts.join(' · ') : 'cap: unlimited';
    }
}

document.addEventListener('DOMContentLoaded', () => {
    window.overviewPage = new OverviewPage();
});
