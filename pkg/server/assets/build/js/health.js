// System health page — consumes GET /api/health
class HealthPage {
    constructor() {
        this.refs = {
            loading: document.getElementById('health-loading'),
            content: document.getElementById('health-content'),
            error: document.getElementById('health-error'),
            errorMsg: document.getElementById('health-error-message'),
            overall: document.getElementById('health-overall-badge'),
            lastChecked: document.getElementById('health-last-checked'),
            debrids: document.getElementById('health-debrids'),
            arrs: document.getElementById('health-arrs'),
            disk: document.getElementById('health-disk'),
            queue: document.getElementById('health-queue'),
            refresh: document.getElementById('refresh-health'),
        };
        this.pollInterval = null;
        this.init();
    }

    init() {
        this.refs.refresh?.addEventListener('click', () => this.load());
        this.load();
        this.pollInterval = setInterval(() => this.load(), 60000);
    }

    async load() {
        this.showLoading();
        try {
            const res = await fetch(`${window.urlBase || ''}api/health`);
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            const data = await res.json();
            this.render(data);
            this.showContent();
        } catch (err) {
            this.showError(err.message || 'Failed to load health');
        }
    }

    showLoading() {
        this.refs.loading?.classList.remove('hidden');
        this.refs.content?.classList.add('hidden');
        this.refs.error?.classList.add('hidden');
    }

    showContent() {
        this.refs.loading?.classList.add('hidden');
        this.refs.error?.classList.add('hidden');
        this.refs.content?.classList.remove('hidden');
    }

    showError(msg) {
        this.refs.loading?.classList.add('hidden');
        this.refs.content?.classList.add('hidden');
        this.refs.error?.classList.remove('hidden');
        if (this.refs.errorMsg) this.refs.errorMsg.textContent = msg;
    }

    statusBadge(status) {
        const map = {
            ok: 'badge-success',
            writable: 'badge-success',
            healthy: 'badge-success',
            degraded: 'badge-warning',
            error: 'badge-error',
            timeout: 'badge-warning',
            unhealthy: 'badge-error',
            unconfigured: 'badge-ghost',
            not_found: 'badge-ghost',
            not_writable: 'badge-error',
            missing: 'badge-error',
        };
        const cls = map[status] || 'badge-ghost';
        return `<span class="badge ${cls} badge-sm">${status}</span>`;
    }

    renderList(container, checks) {
        if (!container) return;
        if (!checks || typeof checks !== 'object' || Object.keys(checks).length === 0) {
            container.innerHTML = '<p class="text-sm opacity-60">No checks configured</p>';
            return;
        }
        container.innerHTML = Object.entries(checks).map(([name, status]) => `
            <div class="flex justify-between items-center py-1 border-b border-base-200 last:border-0">
                <span class="font-medium text-sm">${this.escapeHtml(name)}</span>
                ${this.statusBadge(String(status))}
            </div>
        `).join('');
    }

    render(data) {
        const status = data.status || 'unknown';
        const overallMap = {
            healthy: 'badge-success',
            degraded: 'badge-warning',
            unhealthy: 'badge-error',
        };
        if (this.refs.overall) {
            this.refs.overall.className = `badge badge-lg ${overallMap[status] || 'badge-ghost'}`;
            this.refs.overall.textContent = status;
        }
        if (this.refs.lastChecked) {
            this.refs.lastChecked.textContent = `Last checked: ${new Date().toLocaleString()}`;
        }

        const checks = data.checks || {};
        this.renderList(this.refs.debrids, checks.debrids);
        this.renderList(this.refs.arrs, checks.arrs);
        this.renderList(this.refs.disk, checks.disk);

        const queue = checks.queue || {};
        const stuckCount = queue.stuck_count ?? 0;
        const stuckItems = queue.stuck_items || [];
        const base = window.urlBase || '';
        let queueHtml = `<p class="text-sm"><strong>${stuckCount}</strong> stuck at debrid-complete (&gt;30m)</p>`;
        if (stuckItems.length > 0) {
            queueHtml += '<ul class="mt-2 space-y-1">';
            stuckItems.slice(0, 10).forEach((hash) => {
                queueHtml += `<li class="font-mono text-xs"><a href="${base}?search=${encodeURIComponent(hash)}" class="link link-hover">${hash.substring(0, 16)}…</a></li>`;
            });
            if (stuckItems.length > 10) {
                queueHtml += `<li class="text-xs opacity-60">+${stuckItems.length - 10} more</li>`;
            }
            queueHtml += '</ul>';
        }
        if (this.refs.queue) this.refs.queue.innerHTML = queueHtml;
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}

document.addEventListener('DOMContentLoaded', () => {
    window.healthPage = new HealthPage();
});
