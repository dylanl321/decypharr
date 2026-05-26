// Dashboard functionality for torrent management with server-side operations
class TorrentDashboard {
    constructor() {
        this.state = {
            torrents: [],
            selectedEntries: new Set(),
            categories: [],
            total: 0,
            currentPage: 1,
            itemsPerPage: 20,
            totalPages: 0,
            searchQuery: '',
            selectedCategory: '',
            selectedState: '',
            providerFilter: '',
            sortBy: 'added_on',
            sortOrder: 'desc',
            selectedTorrentContextMenu: null
        };

        this.refs = {
            torrentsList: document.getElementById('torrentsList'),
            searchInput: document.getElementById('searchInput'),
            categoryFilter: document.getElementById('categoryFilter'),
            stateFilter: document.getElementById('stateFilter'),
            sortSelector: document.getElementById('sortSelector'),
            selectAll: document.getElementById('selectAll'),
            batchDeleteBtn: document.getElementById('batchDeleteBtn'),
            batchDeleteDebridBtn: document.getElementById('batchDeleteDebridBtn'),
            refreshBtn: document.getElementById('refreshBtn'),
            torrentContextMenu: document.getElementById('torrentContextMenu'),
            paginationControls: document.getElementById('paginationControls'),
            paginationInfo: document.getElementById('paginationInfo'),
            emptyState: document.getElementById('emptyState'),
            providerChips: document.getElementById('providerChips'),
            providerChipsLoading: document.getElementById('providerChipsLoading'),
            summaryPills: document.getElementById('summaryPills'),
            densityCozy: document.getElementById('densityCozy'),
            densityCompact: document.getElementById('densityCompact'),
        };

        this.searchTimeout = null;
        this.timeline = new TimelineDrawer();
        this.init();
    }

    init() {
        this.applyURLSearch();
        this.applyDensity();
        this.bindEvents();
        this.loadProviderChips();
        this.loadSummary();
        this.loadTorrents();
        this.startAutoRefresh();
    }

    applyDensity() {
        const d = window.getDensity ? window.getDensity() : 'cozy';
        if (this.refs.densityCozy) this.refs.densityCozy.classList.toggle('btn-active', d === 'cozy');
        if (this.refs.densityCompact) this.refs.densityCompact.classList.toggle('btn-active', d === 'compact');
    }

    async loadProviderChips() {
        if (!this.refs.providerChips) return;
        try {
            const res = await window.decypharrUtils.fetcher('/api/debrid/status');
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            const data = await res.json();
            const providers = data.providers || [];
            if (this.refs.providerChipsLoading) this.refs.providerChipsLoading.remove();
            const existing = this.refs.providerChips.querySelectorAll('[data-prov-chip-name]');
            existing.forEach(n => n.remove());
            providers.forEach(p => {
                const slug = window.providerSlug(p.type || p.name);
                const color = window.providerColor(p.type || p.name);
                const chip = document.createElement('span');
                chip.className = 'prov-chip';
                chip.dataset.provFilter = p.name;
                chip.dataset.provChipName = p.name;
                chip.dataset.prov = slug;
                chip.style.setProperty('--prov-color', color);
                chip.innerHTML = `<span class="prov-chip-dot"></span> ${window.decypharrUtils.escapeHtml(p.name)}`;
                this.refs.providerChips.appendChild(chip);
            });
        } catch (err) {
            if (this.refs.providerChipsLoading) {
                this.refs.providerChipsLoading.textContent = 'unavailable';
            }
        }
    }

    async loadSummary() {
        try {
            const res = await window.decypharrUtils.fetcher('/api/queue/summary');
            if (!res.ok) return;
            const data = await res.json();
            const byState = data.by_state || {};
            const total = data.total || 0;
            const setText = (id, val) => {
                const el = document.getElementById(id);
                if (el) el.textContent = String(val || 0);
            };
            setText('pillTotal', total);
            setText('pillDownloading', byState.downloading || 0);
            setText('pillQueued', byState.queued || 0);
            setText('pillPending', byState.pending || 0);
            setText('pillCompleted', byState.pausedUP || 0);
            setText('pillErrors', byState.error || (data.errors ? data.errors.length : 0));
        } catch (err) {
            // Silent fail - summary is best-effort
        }
    }

    applyURLSearch() {
        const params = new URLSearchParams(window.location.search);
        const search = params.get('search');
        if (search && this.refs.searchInput) {
            this.refs.searchInput.value = search;
            this.state.searchQuery = search;
        }
    }

    bindEvents() {
        // Refresh button
        this.refs.refreshBtn.addEventListener('click', () => this.loadTorrents());

        // Batch delete
        this.refs.batchDeleteBtn.addEventListener('click', () => this.deleteSelectedTorrents());
        this.refs.batchDeleteDebridBtn.addEventListener('click', () => this.deleteSelectedTorrents(true));

        // Select all checkbox
        this.refs.selectAll.addEventListener('change', (e) => this.toggleSelectAll(e.target.checked));

        // Search with debounce
        this.refs.searchInput.addEventListener('input', (e) => {
            clearTimeout(this.searchTimeout);
            this.searchTimeout = setTimeout(() => {
                this.state.searchQuery = e.target.value;
                this.state.currentPage = 1;
                this.loadTorrents();
            }, 300);
        });

        // Filters
        this.refs.categoryFilter.addEventListener('change', (e) => {
            this.state.selectedCategory = e.target.value;
            this.state.currentPage = 1;
            this.loadTorrents();
        });

        this.refs.stateFilter.addEventListener('change', (e) => {
            this.state.selectedState = e.target.value;
            this.state.currentPage = 1;
            this.loadTorrents();
        });

        this.refs.sortSelector.addEventListener('change', (e) => {
            const value = e.target.value;
            // Parse sort format: "field" or "field_asc" or "field_desc"
            if (value.endsWith('_asc')) {
                this.state.sortBy = value.replace('_asc', '');
                this.state.sortOrder = 'asc';
            } else if (value.endsWith('_desc')) {
                this.state.sortBy = value.replace('_desc', '');
                this.state.sortOrder = 'desc';
            } else {
                this.state.sortBy = value;
                this.state.sortOrder = 'desc';
            }
            this.state.currentPage = 1;
            this.updateSortIndicators();
            this.loadTorrents();
        });

        // Context menu
        this.bindContextMenu();

        // Sortable column headers
        document.querySelectorAll('.sortable-th').forEach((th) => {
            th.addEventListener('click', () => {
                const field = th.dataset.sort;
                if (!field) return;
                if (this.state.sortBy === field) {
                    this.state.sortOrder = this.state.sortOrder === 'asc' ? 'desc' : 'asc';
                } else {
                    this.state.sortBy = field;
                    this.state.sortOrder = (field === 'name' || field === 'category') ? 'asc' : 'desc';
                }
                this.state.currentPage = 1;
                this.syncSortSelector();
                this.updateSortIndicators();
                this.loadTorrents();
            });
        });
        this.updateSortIndicators();

        // Density toggle
        if (this.refs.densityCozy) {
            this.refs.densityCozy.addEventListener('click', () => {
                window.setDensity('cozy');
                this.applyDensity();
            });
        }
        if (this.refs.densityCompact) {
            this.refs.densityCompact.addEventListener('click', () => {
                window.setDensity('compact');
                this.applyDensity();
            });
        }

        // Provider chip click
        if (this.refs.providerChips) {
            this.refs.providerChips.addEventListener('click', (e) => {
                const chip = e.target.closest('[data-prov-filter]');
                if (!chip) return;
                this.state.providerFilter = chip.dataset.provFilter || '';
                this.refs.providerChips.querySelectorAll('[data-prov-filter]').forEach(c =>
                    c.setAttribute('data-active', c === chip ? 'true' : 'false'));
                this.renderTorrents();
            });
        }

        // Summary pills
        if (this.refs.summaryPills) {
            this.refs.summaryPills.addEventListener('click', (e) => {
                const btn = e.target.closest('[data-state-pill]');
                if (!btn) return;
                this.state.selectedState = btn.dataset.statePill || '';
                if (this.refs.stateFilter) this.refs.stateFilter.value = this.state.selectedState;
                this.refs.summaryPills.querySelectorAll('[data-state-pill]').forEach(b =>
                    b.setAttribute('data-active', b === btn ? 'true' : 'false'));
                this.state.currentPage = 1;
                this.loadTorrents();
            });
        }

        // Torrent selection
        this.refs.torrentsList.addEventListener('change', (e) => {
            if (e.target.classList.contains('torrent-select')) {
                this.toggleTorrentSelection(e.target.dataset.hash, e.target.checked);
            }
        });

        // Click row name to open timeline drawer
        this.refs.torrentsList.addEventListener('click', (e) => {
            const trigger = e.target.closest('[data-action="open-timeline"]');
            if (!trigger) return;
            const row = trigger.closest('tr[data-hash]');
            if (!row) return;
            this.timeline.open({ hash: row.dataset.hash, name: row.dataset.name, debrid: row.dataset.prov });
        });
    }

    bindContextMenu() {
        // Show context menu
        this.refs.torrentsList.addEventListener('contextmenu', (e) => {
            const row = e.target.closest('tr[data-hash]');
            if (!row) return;

            e.preventDefault();
            this.showContextMenu(e, row);
        });

        // Hide context menu
        document.addEventListener('click', (e) => {
            if (!this.refs.torrentContextMenu.contains(e.target)) {
                this.hideContextMenu();
            }
        });

        // Context menu actions
        this.refs.torrentContextMenu.addEventListener('click', (e) => {
            const action = e.target.closest('[data-action]')?.dataset.action;
            if (action) {
                this.handleContextAction(action);
                this.hideContextMenu();
            }
        });
    }

    showContextMenu(event, row) {
        this.state.selectedTorrentContextMenu = {
            hash: row.dataset.hash,
            name: row.dataset.name,
            category: row.dataset.category || '',
            debrid: row.dataset.prov || ''
        };

        this.refs.torrentContextMenu.querySelector('.torrent-name').textContent =
            this.state.selectedTorrentContextMenu.name;

        const {pageX, pageY} = event;
        const {clientWidth, clientHeight} = document.documentElement;
        const menu = this.refs.torrentContextMenu;

        // Position the menu
        menu.style.left = `${Math.min(pageX, clientWidth - 200)}px`;
        menu.style.top = `${Math.min(pageY, clientHeight - 150)}px`;

        menu.classList.remove('hidden');
    }

    hideContextMenu() {
        this.refs.torrentContextMenu.classList.add('hidden');
        this.state.selectedTorrentContextMenu = null;
    }

    async handleContextAction(action) {
        const torrent = this.state.selectedTorrentContextMenu;
        if (!torrent) return;

        const actions = {
            'open-timeline': async () => {
                this.timeline.open({ hash: torrent.hash, name: torrent.name, debrid: torrent.debrid || torrent.active_provider });
            },
            'copy-magnet': async () => {
                try {
                    await navigator.clipboard.writeText(`magnet:?xt=urn:btih:${torrent.hash}`);
                    window.decypharrUtils.createToast('Magnet link copied to clipboard');
                } catch (error) {
                    window.decypharrUtils.createToast('Failed to copy magnet link', 'error');
                }
            },
            'copy-name': async () => {
                try {
                    await navigator.clipboard.writeText(torrent.name);
                    window.decypharrUtils.createToast('Torrent name copied to clipboard');
                } catch (error) {
                    window.decypharrUtils.createToast('Failed to copy torrent name', 'error');
                }
            },
            'delete': async () => {
                await this.deleteTorrent(torrent.hash, torrent.category, false);
            },
            'delete-debrid': async () => {
                await this.deleteTorrent(torrent.hash, torrent.category, true);
            }
        };

        if (actions[action]) {
            await actions[action]();
        }
    }

    async loadTorrents() {
        try {
            // Show loading state
            this.refs.refreshBtn.disabled = true;
            this.refs.paginationInfo.textContent = 'Loading torrents...';
            // Build query parameters
            const params = new URLSearchParams({
                page: this.state.currentPage,
                limit: this.state.itemsPerPage,
                sort_by: this.state.sortBy,
                sort_order: this.state.sortOrder
            });

            if (this.state.searchQuery) {
                params.set('search', this.state.searchQuery);
            }

            if (this.state.selectedCategory) {
                params.set('category', this.state.selectedCategory);
            }

            if (this.state.selectedState) {
                params.set('state', this.state.selectedState);
            }

            const response = await window.decypharrUtils.fetcher(`/api/torrents?${params}`);
            if (!response.ok) throw new Error('Failed to fetch items');

            const data = await response.json();
            this.state.torrents = data.torrents || [];
            this.state.total = data.total || 0;
            this.state.totalPages = data.total_pages || 0;
            this.state.categories = data.categories || [];

            this.updateUI();

        } catch (error) {
            console.error('Error loading items:', error);
            window.decypharrUtils.createToast(`Error loading items: ${error.message}`, 'error');
        } finally {
            this.refs.refreshBtn.disabled = false;
        }
    }

    updateUI() {
        // Update category dropdown
        this.updateCategoryFilter();

        // Render torrents table
        this.renderTorrents();

        // Update pagination
        this.renderPagination();

        // Update selection state
        this.updateSelectionUI();

        // Show/hide empty state
        this.toggleEmptyState();
    }

    updateCategoryFilter() {
        const currentValue = this.refs.categoryFilter.value;
        this.refs.categoryFilter.innerHTML = '<option value="">All Categories</option>';

        this.state.categories.forEach(category => {
            const option = document.createElement('option');
            option.value = category;
            option.textContent = category;
            if (category === currentValue) {
                option.selected = true;
            }
            this.refs.categoryFilter.appendChild(option);
        });
    }

    renderTorrents() {
        const list = this.state.providerFilter
            ? this.state.torrents.filter(t => ((t.debrid || t.active_provider || '')) === this.state.providerFilter)
            : this.state.torrents;

        if (list.length === 0) {
            this.refs.torrentsList.innerHTML = '';
            return;
        }

        this.refs.torrentsList.innerHTML = list.map(torrent => {
            const isSelected = this.state.selectedEntries.has(torrent.info_hash);
            const provider = torrent.debrid || torrent.active_provider || '';
            const provSlug = window.providerSlug(provider);
            const provColor = window.providerColor(provider);
            const protoIcon = torrent.protocol === 'nzb' ? 'bi-newspaper' : 'bi-magnet';
            return `
                <tr class="hover prov-stripe" data-hash="${torrent.info_hash}"
                    data-name="${this.escapeHtml(torrent.name)}"
                    data-category="${this.escapeHtml(torrent.category || '')}"
                    data-prov="${provSlug}"
                    style="--prov-color:${provColor}">
                    <td>
                        <label class="cursor-pointer">
                            <input type="checkbox" class="checkbox checkbox-sm checkbox-primary torrent-select"
                                   data-hash="${torrent.info_hash}" ${isSelected ? 'checked' : ''}>
                        </label>
                    </td>
                    <td>
                        <div class="flex flex-col gap-0.5">
                            <button type="button" class="font-medium text-left link link-hover" data-action="open-timeline">
                                <i class="bi ${protoIcon} text-base-content/50 mr-1"></i>${this.escapeHtml(torrent.name)}
                            </button>
                            <span class="text-xs text-base-content/60 font-mono">${torrent.info_hash.substring(0, 8)}…</span>
                        </div>
                    </td>
                    <td>
                        ${provider ? `
                            <span class="prov-chip" data-prov="${provSlug}" data-active="true"
                                  style="--prov-color:${provColor}">
                                <span class="prov-chip-dot"></span>${this.escapeHtml(provider)}
                            </span>` : '<span class="text-xs opacity-50">—</span>'}
                    </td>
                    <td class="size-cell">
                        <span class="text-sm font-mono whitespace-nowrap">${this.formatSize(torrent.size)}</span>
                    </td>
                    <td>
                        ${this.renderProgressCell(torrent)}
                    </td>
                    <td>
                        <span class="text-sm font-mono whitespace-nowrap">${this.renderSpeed(torrent)}</span>
                    </td>
                    <td>
                        ${torrent.category ? `<span class="badge badge-sm badge-outline">${this.escapeHtml(torrent.category)}</span>` : '-'}
                    </td>
                    <td>
                        ${this.renderStatusCell(torrent)}
                    </td>
                    <td>
                        <button class="btn btn-ghost btn-xs"
                                title="View History"
                                data-action="open-timeline">
                            <i class="bi bi-clock-history"></i>
                        </button>
                        ${torrent.state === 'pending' ? `
                            <button class="btn btn-ghost btn-xs text-info"
                                    title="Retry Now"
                                    onclick="window.dashboard.retryPendingEntry('${torrent.info_hash}');">
                                <i class="bi bi-arrow-clockwise"></i>
                            </button>
                        ` : ''}
                        <button class="btn btn-ghost btn-xs text-error"
                                title="Delete Torrent"
                                onclick="window.dashboard.deleteTorrent('${torrent.info_hash}', '${this.escapeAttr(torrent.category || '')}', false);">
                            <i class="bi bi-trash"></i>
                        </button>
                        ${torrent.state !== 'pending' ? `
                            <button class="btn btn-ghost btn-xs text-error"
                                    title="Delete from Provider"
                                    onclick="window.dashboard.deleteTorrent('${torrent.info_hash}', '${this.escapeAttr(torrent.category || '')}', true);">
                                <i class="bi bi-cloud-slash"></i>
                            </button>
                        ` : ''}
                    </td>
                </tr>
            `;
        }).join('');
    }

    renderStatusCell(torrent) {
        const stateMap = {
            'pausedUP': { class: 'badge-success', text: 'Completed', icon: 'bi-check-circle' },
            'downloading': { class: 'badge-info', text: 'Downloading', icon: 'bi-arrow-down-circle' },
            'error': { class: 'badge-error', text: 'Error', icon: 'bi-exclamation-triangle' },
            'queued': { class: 'badge-ghost', text: 'Queued', icon: 'bi-hourglass-split' },
            'paused': { class: 'badge-warning', text: 'Paused', icon: 'bi-pause-circle' },
            'pending': { class: 'badge-warning', text: 'Pending', icon: 'bi-clock-history' }
        };
        const s = stateMap[torrent.state] || { class: 'badge-ghost', text: torrent.state || 'unknown', icon: 'bi-question' };
        const phaseLabels = {
            queued: 'Queued',
            debrid_fetching: 'Provider',
            downloading: 'Local pull',
            importing: 'Importing',
            complete: 'Complete'
        };
        const phase = torrent.phase ? (phaseLabels[torrent.phase] || torrent.phase) : '';
        const reason = torrent.pending_reason || torrent.last_error || torrent.state || '';
        return `
            <div class="tooltip tooltip-left" data-tip="${this.escapeAttr(reason)}">
                <span class="badge ${s.class} badge-sm gap-1"><i class="bi ${s.icon}"></i>${s.text}</span>
                ${phase ? `<span class="text-[10px] block opacity-60 mt-0.5">${this.escapeHtml(phase)}</span>` : ''}
                ${torrent.state === 'pending' && torrent.pending_reason ? `<span class="text-[10px] block opacity-60 mt-0.5">${this.escapeHtml(torrent.pending_reason)}</span>` : ''}
            </div>
        `;
    }

    renderPhaseBadge(phase) {
        if (!phase) return '<span class="text-xs opacity-50">—</span>';
        const labels = {
            queued: 'Queued',
            debrid_fetching: 'Debrid',
            downloading: 'Pulling',
            importing: 'Import',
            complete: 'Done',
        };
        const text = labels[phase] || phase;
        return `<span class="badge badge-outline badge-xs" title="${this.escapeAttr(phase)}">${text}</span>`;
    }

    renderProgressCell(torrent) {
        const state = torrent.state;
        if (state === 'error') {
            return `<span class="text-xs opacity-60">—</span>`;
        }
        if (state === 'pausedUP') {
            return `
                <div class="flex items-center gap-2">
                    <progress class="progress progress-success w-20" value="100" max="100"></progress>
                    <span class="text-xs font-medium">100%</span>
                </div>
            `;
        }
        const hasSplit = (torrent.debrid_progress > 0 || torrent.local_progress > 0) &&
            (torrent.debrid_progress < 1 || torrent.local_progress < 1);
        if (hasSplit) {
            const debridPct = Math.round((torrent.debrid_progress || 0) * 100);
            const localPct = Math.round((torrent.local_progress || 0) * 100);
            const overall = Math.round((torrent.progress || 0) * 100);
            return `
                <div class="tooltip tooltip-top" data-tip="Debrid ${debridPct}% · Local ${localPct}% · Overall ${overall}%">
                    <div class="flex flex-col gap-0.5 w-24">
                        <div class="flex items-center gap-1">
                            <span class="text-[10px] w-8 opacity-60">D</span>
                            <progress class="progress progress-info h-1.5 flex-1" value="${debridPct}" max="100"></progress>
                        </div>
                        <div class="flex items-center gap-1">
                            <span class="text-[10px] w-8 opacity-60">L</span>
                            <progress class="progress progress-secondary h-1.5 flex-1" value="${localPct}" max="100"></progress>
                        </div>
                        <span class="text-xs font-medium text-center">${overall}%</span>
                    </div>
                </div>
            `;
        }
        return this.renderProgressBar(torrent.progress);
    }

    renderSpeed(torrent) {
        if (torrent.state !== 'downloading') return '—';
        const bps = torrent.speed ?? torrent.dlspeed;
        if (!bps) return '—';
        return this.formatSpeed(bps);
    }

    renderProgressBar(progress) {
        const percent = Math.round((progress || 0) * 100);
        let color = 'progress-info';
        if (percent === 100) color = 'progress-success';
        else if (percent < 25) color = 'progress-error';
        else if (percent < 75) color = 'progress-warning';

        return `
            <div class="flex items-center gap-2">
                <progress class="progress ${color} w-20" value="${percent}" max="100"></progress>
                <span class="text-xs font-medium">${percent}%</span>
            </div>
        `;
    }

    renderStateBadge(state) {
        const stateMap = {
            'pausedUP': {class: 'badge-success', text: 'Completed'},
            'downloading': {class: 'badge-info', text: 'Downloading'},
            'error': {class: 'badge-error', text: 'Error'},
            'queued': {class: 'badge-ghost', text: 'Queued'},
            'paused': {class: 'badge-warning', text: 'Paused'}
        };

        const s = stateMap[state] || {class: 'badge-ghost', text: state};
        return `<span class="badge ${s.class} badge-sm">${s.text}</span>`;
    }

    renderProtocolBadge(protocol) {
        const protocolMap = {
            'torrent': {class: 'badge-accent', icon: 'bi-magnet', text: 'Torrent'},
            'nzb': {class: 'badge-secondary', icon: 'bi-newspaper', text: 'Usenet'}
        };

        const p = protocolMap[protocol] || {
            class: 'badge-ghost',
            icon: 'bi-question-circle',
            text: protocol || 'Unknown'
        };
        return `<span class="badge ${p.class} badge-sm"><i class="${p.icon} mr-1"></i>${p.text}</span>`;
    }

    renderPagination() {
        const start = (this.state.currentPage - 1) * this.state.itemsPerPage + 1;
        const end = Math.min(start + this.state.itemsPerPage - 1, this.state.total);

        this.refs.paginationInfo.textContent =
            this.state.total > 0 ?
                `Showing ${start}-${end} of ${this.state.total} items` :
                'No items found';

        if (this.state.totalPages <= 1) {
            this.refs.paginationControls.innerHTML = '';
            return;
        }

        let html = `
            <button class="join-item btn btn-sm ${this.state.currentPage === 1 ? 'btn-disabled' : ''}"
                    onclick="window.dashboard.goToPage(${this.state.currentPage - 1});">«</button>
        `;

        for (let i = 1; i <= this.state.totalPages; i++) {
            if (i === 1 || i === this.state.totalPages ||
                (i >= this.state.currentPage - 2 && i <= this.state.currentPage + 2)) {
                html += `
                    <button class="join-item btn btn-sm ${i === this.state.currentPage ? 'btn-active' : ''}"
                            onclick="window.dashboard.goToPage(${i});">${i}</button>
                `;
            } else if (i === this.state.currentPage - 3 || i === this.state.currentPage + 3) {
                html += `<button class="join-item btn btn-sm btn-disabled">...</button>`;
            }
        }

        html += `
            <button class="join-item btn btn-sm ${this.state.currentPage === this.state.totalPages ? 'btn-disabled' : ''}"
                    onclick="window.dashboard.goToPage(${this.state.currentPage + 1})">»</button>
        `;

        this.refs.paginationControls.innerHTML = html;
    }

    goToPage(page) {
        if (page < 1 || page > this.state.totalPages) return;
        this.state.currentPage = page;
        this.loadTorrents();
    }

    toggleEmptyState() {
        const hasResults = this.state.total > 0;
        this.refs.emptyState.classList.toggle('hidden', hasResults);
        this.refs.torrentsList.closest('.card').classList.toggle('hidden', !hasResults);
    }

    toggleSelectAll(checked) {
        if (checked) {
            this.state.torrents.forEach(t => this.state.selectedEntries.add(t.info_hash));
        } else {
            this.state.selectedEntries.clear();
        }
        this.renderTorrents();
        this.updateSelectionUI();
    }

    toggleTorrentSelection(hash, checked) {
        if (checked) {
            this.state.selectedEntries.add(hash);
        } else {
            this.state.selectedEntries.delete(hash);
        }
        this.updateSelectionUI();
    }

    updateSelectionUI() {
        const hasSelection = this.state.selectedEntries.size > 0;
        this.refs.batchDeleteBtn.classList.toggle('hidden', !hasSelection);
        this.refs.batchDeleteDebridBtn.classList.toggle('hidden', !hasSelection);

        const allSelected = this.state.torrents.length > 0 &&
            this.state.torrents.every(t => this.state.selectedEntries.has(t.info_hash));
        this.refs.selectAll.checked = allSelected;
    }

    async retryPendingEntry(hash) {
        try {
            const url = `${window.urlBase}api/queue/${hash}/retry`;
            const response = await window.decypharrUtils.fetcher(url, {method: 'POST'});

            if (!response.ok) throw new Error('Failed to retry entry');

            window.decypharrUtils.createToast('Retry scheduled successfully');
            this.loadTorrents();
        } catch (error) {
            console.error('Error retrying entry:', error);
            window.decypharrUtils.createToast('Failed to retry entry', 'error');
        }
    }

    async deleteTorrent(hash, category, removeFromDebrid = false) {
        if (!confirm('Are you sure you want to delete this torrent?')) return;

        try {
            const url = `${window.urlBase}api/torrents/${category}/${hash}?removeFromDebrid=${removeFromDebrid}`;
            const response = await window.decypharrUtils.fetcher(url, {method: 'DELETE'});

            if (!response.ok) throw new Error('Failed to delete entry');

            window.decypharrUtils.createToast('Item deleted successfully');
            this.state.selectedEntries.delete(hash);
            this.loadTorrents();
        } catch (error) {
            console.error('Error deleting torrent:', error);
            window.decypharrUtils.createToast('Failed to delete entry', 'error');
        }
    }

    async deleteSelectedTorrents(removeFromDebrid = false) {
        if (this.state.selectedEntries.size === 0) return;

        if (!confirm(`Delete ${this.state.selectedEntries.size} selected items?`)) return;

        try {
            const hashes = Array.from(this.state.selectedEntries).join(',');
            const url = `${window.urlBase}api/torrents?hashes=${hashes}&removeFromDebrid=${removeFromDebrid}`;
            const response = await window.decypharrUtils.fetcher(url, {method: 'DELETE'});

            if (!response.ok) throw new Error('Failed to delete items');

            window.decypharrUtils.createToast(`Deleted ${this.state.selectedEntries.size} items successfully`);
            this.state.selectedEntries.clear();
            this.loadTorrents();
        } catch (error) {
            console.error('Error deleting items:', error);
            window.decypharrUtils.createToast('Failed to delete items', 'error');
        }
    }

    startAutoRefresh() {
        setInterval(() => {
            this.loadTorrents();
            this.loadSummary();
        }, 10000); // Refresh every 10 seconds
    }

    updateSortIndicators() {
        document.querySelectorAll('.sortable-th').forEach((th) => {
            const field = th.dataset.sort;
            const indicator = th.querySelector('.sort-indicator');
            if (!indicator) return;
            indicator.classList.remove('bi-caret-up-fill', 'bi-caret-down-fill', 'bi-arrow-down-up');
            if (field === this.state.sortBy) {
                indicator.classList.add(this.state.sortOrder === 'asc' ? 'bi-caret-up-fill' : 'bi-caret-down-fill');
                th.setAttribute('data-active', 'true');
            } else {
                indicator.classList.add('bi-arrow-down-up');
                th.removeAttribute('data-active');
            }
        });
    }

    syncSortSelector() {
        if (!this.refs.sortSelector) return;
        const { sortBy, sortOrder } = this.state;
        const optionWithSuffix = `${sortBy}_${sortOrder}`;
        const opts = Array.from(this.refs.sortSelector.options).map((o) => o.value);
        if (opts.includes(optionWithSuffix)) {
            this.refs.sortSelector.value = optionWithSuffix;
        } else if (opts.includes(sortBy)) {
            this.refs.sortSelector.value = sortBy;
        }
    }

    // Utility methods
    formatSize(bytes) {
        if (!bytes || bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }

    formatSpeed(bytesPerSec) {
        if (!bytesPerSec || bytesPerSec === 0) return '-';
        return this.formatSize(bytesPerSec) + '/s';
    }

    escapeHtml(text) {
        if (!text) return '';
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    escapeAttr(text) {
        if (!text) return '';
        return text.replace(/'/g, '&#39;').replace(/"/g, '&quot;');
    }
}

// =====================================================================
// Timeline drawer (history of an entry)
// =====================================================================
class TimelineDrawer {
    constructor() {
        this.drawer = document.getElementById('timelineDrawer');
        this.backdrop = document.getElementById('timelineDrawerBackdrop');
        this.body = document.getElementById('timelineDrawerBody');
        this.entryName = document.getElementById('timelineEntryName');
        this.entryMeta = document.getElementById('timelineEntryMeta');
        this.copyBtn = document.getElementById('timelineCopyBtn');
        this.closeBtn = document.getElementById('timelineCloseBtn');
        this.currentHash = null;
        this.events = [];
        this.refreshTimer = null;
        if (!this.drawer) return;

        this.closeBtn?.addEventListener('click', () => this.close());
        this.backdrop?.addEventListener('click', () => this.close());
        this.copyBtn?.addEventListener('click', () => this.copyAsText());
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape' && this.drawer.classList.contains('open')) this.close();
        });
    }

    async open({ hash, name, debrid }) {
        if (!this.drawer) return;
        this.currentHash = hash;
        this.entryName.textContent = name || hash;
        this.entryMeta.textContent = debrid ? `Provider: ${debrid}` : '—';
        this.drawer.classList.add('open');
        this.drawer.setAttribute('aria-hidden', 'false');
        this.backdrop.classList.add('open');
        this.body.innerHTML = `<div class="text-center py-8 opacity-60"><span class="loading loading-spinner loading-sm"></span></div>`;
        await this.loadEvents();
        this.refreshTimer = setInterval(() => this.loadEvents(), 5000);
    }

    close() {
        if (!this.drawer) return;
        this.drawer.classList.remove('open');
        this.drawer.setAttribute('aria-hidden', 'true');
        this.backdrop.classList.remove('open');
        this.currentHash = null;
        if (this.refreshTimer) {
            clearInterval(this.refreshTimer);
            this.refreshTimer = null;
        }
    }

    async loadEvents() {
        if (!this.currentHash) return;
        try {
            const res = await window.decypharrUtils.fetcher(`/api/queue/${this.currentHash}/timeline`);
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            const data = await res.json();
            this.events = Array.isArray(data) ? data : (data.timeline || []);
            this.render();
        } catch (err) {
            this.body.innerHTML = `<div class="alert alert-error alert-sm text-xs">Failed to load timeline: ${window.decypharrUtils.escapeHtml(err.message)}</div>`;
        }
    }

    render() {
        if (!this.events.length) {
            this.body.innerHTML = `<div class="opacity-60 text-sm text-center py-8">No events recorded yet for this entry.</div>`;
            return;
        }
        const sorted = this.events.slice().sort((a, b) =>
            new Date(a.at).getTime() - new Date(b.at).getTime());
        const items = sorted.map(ev => this.renderItem(ev)).join('');
        this.body.innerHTML = `<ol class="timeline-list">${items}</ol>`;
    }

    renderItem(ev) {
        const at = new Date(ev.at);
        const abs = at.toLocaleString();
        const rel = this.relTime(at);
        const meta = [];
        if (ev.bytes) meta.push(window.decypharrUtils.formatBytes(ev.bytes));
        if (ev.duration) meta.push(this.formatDuration(ev.duration));
        if (ev.provider) meta.push(`provider: ${ev.provider}`);
        const icon = this.iconFor(ev.kind);
        return `
            <li class="timeline-item" data-kind="${window.decypharrUtils.escapeHtml(ev.kind)}">
                <div class="timeline-marker"><i class="bi ${icon}"></i></div>
                <div class="timeline-kind">${this.labelFor(ev.kind)}</div>
                <div class="timeline-time" title="${window.decypharrUtils.escapeHtml(abs)}">${rel} · ${window.decypharrUtils.escapeHtml(abs)}</div>
                ${ev.message ? `<div class="timeline-message">${window.decypharrUtils.escapeHtml(ev.message)}</div>` : ''}
                ${meta.length ? `<div class="timeline-meta">${window.decypharrUtils.escapeHtml(meta.join(' · '))}</div>` : ''}
            </li>
        `;
    }

    iconFor(kind) {
        const map = {
            added: 'bi-plus-circle',
            queued: 'bi-hourglass-split',
            debrid_submitted: 'bi-cloud-upload',
            debrid_ready: 'bi-cloud-check',
            provider_blocked: 'bi-shield-exclamation',
            provider_skipped: 'bi-skip-forward',
            local_download_start: 'bi-arrow-down-circle',
            local_download_done: 'bi-check-circle',
            symlinked: 'bi-link-45deg',
            imported: 'bi-box-arrow-in-down',
            error: 'bi-exclamation-triangle',
            removed: 'bi-trash',
        };
        return map[kind] || 'bi-circle';
    }

    labelFor(kind) {
        const map = {
            added: 'Added',
            queued: 'Queued',
            debrid_submitted: 'Submitted to provider',
            debrid_ready: 'Ready on provider',
            provider_blocked: 'Provider blocked (DMCA)',
            provider_skipped: 'Provider skipped',
            local_download_start: 'Local download started',
            local_download_done: 'Local download finished',
            symlinked: 'Symlinked',
            imported: 'Imported by Arr',
            error: 'Error',
            removed: 'Removed',
        };
        return map[kind] || kind;
    }

    relTime(date) {
        const diff = Date.now() - date.getTime();
        if (diff < 60_000) return `${Math.max(1, Math.round(diff / 1000))}s ago`;
        if (diff < 3_600_000) return `${Math.round(diff / 60_000)}m ago`;
        if (diff < 86_400_000) return `${Math.round(diff / 3_600_000)}h ago`;
        return `${Math.round(diff / 86_400_000)}d ago`;
    }

    formatDuration(ns) {
        const s = ns / 1e9;
        if (s < 60) return `${s.toFixed(1)}s`;
        if (s < 3600) return `${Math.floor(s / 60)}m ${Math.round(s % 60)}s`;
        return `${Math.floor(s / 3600)}h ${Math.round((s % 3600) / 60)}m`;
    }

    async copyAsText() {
        if (!this.events.length) return;
        const text = this.events.slice()
            .sort((a, b) => new Date(a.at).getTime() - new Date(b.at).getTime())
            .map(ev => {
                const at = new Date(ev.at).toISOString();
                const parts = [at, this.labelFor(ev.kind)];
                if (ev.message) parts.push(ev.message);
                return parts.join(' — ');
            }).join('\n');
        await window.decypharrUtils.copyToClipboard(text);
    }
}
