// Aether Web UI — Application Logic
// Pure vanilla JS, no frameworks, no dependencies.

(function () {
    'use strict';

    const API_BASE = window.location.origin;

    // --- State ---
    let currentTab = 'timeline';
    let timelineMessages = [];
    let timelineOffset = 0;
    let hasMore = false;
    let sseSource = null;
    let statusInterval = null;

    // --- DOM refs ---
    const $ = (id) => document.getElementById(id);

    const els = {
        statusPill: $('statusPill'),
        timeline: $('timeline'),
        timelineEmpty: $('timelineEmpty'),
        loadMoreBtn: $('loadMoreBtn'),
        liveIndicator: $('liveIndicator'),
        composeInput: $('composeInput'),
        charCount: $('charCount'),
        postBtn: $('postBtn'),
        postResult: $('postResult'),
        statMessages: $('statMessages'),
        statPeers: $('statPeers'),
        statTransport: $('statTransport'),
        statVersion: $('statVersion'),
        statListen: $('statListen'),
        statAdvertise: $('statAdvertise'),
        peerList: $('peerList'),
        peerCountBadge: $('peerCountBadge'),
    };

    // --- Navigation ---
    document.querySelectorAll('.nav-btn').forEach(btn => {
        btn.addEventListener('click', () => switchTab(btn.dataset.tab));
    });

    function switchTab(tab) {
        currentTab = tab;
        document.querySelectorAll('.nav-btn').forEach(b => b.classList.remove('active'));
        document.querySelector(`[data-tab="${tab}"]`).classList.add('active');
        document.querySelectorAll('.panel').forEach(p => p.classList.remove('active'));

        const panelMap = {
            timeline: 'panelTimeline',
            compose: 'panelCompose',
            status: 'panelStatus',
            peers: 'panelPeers',
        };
        $(panelMap[tab]).classList.add('active');

        if (tab === 'status') fetchStatus();
        if (tab === 'peers') fetchPeers();
    }

    // --- Safe JSON fetch (handles non-JSON responses gracefully) ---
    async function safeJSON(url, opts) {
        try {
            const res = await fetch(url, opts);
            const text = await res.text();
            if (!text || text.trimStart().startsWith('<')) return { ok: res.ok, data: null };
            return { ok: res.ok, data: JSON.parse(text) };
        } catch (e) {
            return { ok: false, data: null };
        }
    }

    // --- Timeline ---
    async function fetchTimeline(offset = 0, limit = 50) {
        const { ok, data } = await safeJSON(`${API_BASE}/api/timeline?offset=${offset}&limit=${limit}`);
        if (!ok || !data) return null;
        return data;
    }

    async function loadTimeline() {
        const data = await fetchTimeline(0, 50);
        if (!data) return;

        timelineMessages = data.messages || [];
        timelineOffset = data.next_offset;
        hasMore = data.has_more;

        renderTimeline();
        updateConnectionStatus(true);
    }

    async function loadMore() {
        if (!hasMore) return;
        const data = await fetchTimeline(0, timelineOffset + 50);
        if (!data) return;

        timelineMessages = data.messages || [];
        timelineOffset = data.next_offset;
        hasMore = data.has_more;
        renderTimeline();
    }

    function renderTimeline() {
        if (timelineMessages.length === 0) {
            els.timelineEmpty.style.display = 'block';
            els.loadMoreBtn.style.display = 'none';
            return;
        }

        els.timelineEmpty.style.display = 'none';
        els.loadMoreBtn.style.display = hasMore ? 'block' : 'none';

        // Render newest first (reverse chronological)
        const reversed = [...timelineMessages].reverse();
        const existingHashes = new Set();

        // Keep track of current cards
        const currentCards = els.timeline.querySelectorAll('.msg-card');
        currentCards.forEach(c => existingHashes.add(c.dataset.hash));

        // Clear and re-render
        const frag = document.createDocumentFragment();
        reversed.forEach(msg => {
            frag.appendChild(createMessageCard(msg, false));
        });

        // Preserve empty element
        els.timeline.innerHTML = '';
        els.timeline.appendChild(els.timelineEmpty);
        els.timelineEmpty.style.display = 'none';
        els.timeline.appendChild(frag);
    }

    function createMessageCard(msg, isNew) {
        const card = document.createElement('div');
        card.className = 'msg-card' + (isNew ? ' new' : '');
        card.dataset.hash = msg.hash;

        const ts = new Date(msg.timestamp * 1000);
        const timeStr = ts.toLocaleString();
        const ago = msg.time_ago || relativeTime(msg.timestamp);

        card.innerHTML = `
            <div class="msg-meta">
                <span>${timeStr} · ${ago}</span>
                <span class="msg-hash">#${msg.hash.substring(0, 12)}…</span>
            </div>
            <div class="msg-content">${escapeHtml(msg.content)}</div>
        `;
        return card;
    }

    function prependMessage(msg) {
        // Check if already in timeline
        const exists = timelineMessages.find(m => m.hash === msg.hash);
        if (exists) return;

        timelineMessages.push(msg);
        els.timelineEmpty.style.display = 'none';

        const card = createMessageCard(msg, true);
        // Insert after the empty div
        const firstCard = els.timeline.querySelector('.msg-card');
        if (firstCard) {
            els.timeline.insertBefore(card, firstCard);
        } else {
            els.timeline.appendChild(card);
        }

        // Remove 'new' highlight after animation
        setTimeout(() => card.classList.remove('new'), 3000);
    }

    // --- SSE Stream ---
    function connectSSE() {
        if (sseSource) {
            sseSource.close();
        }

        sseSource = new EventSource(`${API_BASE}/api/timeline/stream`);

        sseSource.onopen = () => {
            els.liveIndicator.classList.add('active');
        };

        sseSource.onmessage = (event) => {
            try {
                const msg = JSON.parse(event.data);
                prependMessage(msg);
            } catch (e) {
                console.error('SSE parse error:', e);
            }
        };

        sseSource.onerror = () => {
            els.liveIndicator.classList.remove('active');
            // Auto-reconnect is built into EventSource
        };
    }

    // --- Compose ---
    els.composeInput.addEventListener('input', () => {
        const len = new TextEncoder().encode(els.composeInput.value).length;
        els.charCount.textContent = len;

        const counter = els.charCount.parentElement;
        counter.classList.remove('warn', 'full');
        if (len >= 320) counter.classList.add('full');
        else if (len >= 280) counter.classList.add('warn');
    });

    els.postBtn.addEventListener('click', postMessage);

    async function postMessage() {
        const content = els.composeInput.value.trim();
        if (!content) return;

        const btnText = els.postBtn.querySelector('.post-btn-text');
        const btnMining = els.postBtn.querySelector('.post-btn-mining');

        els.postBtn.disabled = true;
        btnText.style.display = 'none';
        btnMining.style.display = 'inline';
        els.postResult.style.display = 'none';

        try {
            const { ok, data } = await safeJSON(`${API_BASE}/api/post`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ content }),
            });

            if (!data) throw new Error('API not available — is the node running?');

            if (ok && data.ok) {
                els.postResult.className = 'post-result success';
                els.postResult.textContent = `✓ Broadcast successful — #${data.message.hash.substring(0, 16)}…`;
                els.postResult.style.display = 'block';
                els.composeInput.value = '';
                els.charCount.textContent = '0';
                els.charCount.parentElement.classList.remove('warn', 'full');

                // Switch to timeline to see the message
                setTimeout(() => switchTab('timeline'), 1500);
            } else {
                throw new Error(data.message || res.statusText || 'Unknown error');
            }
        } catch (e) {
            els.postResult.className = 'post-result error';
            els.postResult.textContent = `✕ ${e.message}`;
            els.postResult.style.display = 'block';
        } finally {
            els.postBtn.disabled = false;
            btnText.style.display = 'inline';
            btnMining.style.display = 'none';
        }
    }

    // --- Status ---
    async function fetchStatus() {
        const { ok, data } = await safeJSON(`${API_BASE}/api/status`);
        if (!ok || !data) {
            updateConnectionStatus(false);
            return;
        }

        els.statMessages.textContent = data.message_count.toLocaleString();
        els.statPeers.textContent = data.peer_count.toLocaleString();
        els.statVersion.textContent = data.node_version;
        els.statListen.textContent = data.listen_address || '—';
        els.statAdvertise.textContent = data.advertise_address || '—';

        if (data.dev_clearnet) {
            els.statTransport.textContent = 'Clearnet';
            els.statTransport.style.color = 'var(--yellow)';
        } else if (data.tor_required) {
            els.statTransport.textContent = 'Tor';
            els.statTransport.style.color = 'var(--green)';
        } else {
            els.statTransport.textContent = 'Mixed';
        }

        updateConnectionStatus(true);
    }

    // --- Peers ---
    async function fetchPeers() {
        const { ok, data } = await safeJSON(`${API_BASE}/api/peers`);
        if (!ok || !data) return;

        const peers = data.peers || [];
        els.peerCountBadge.textContent = peers.length;

        if (peers.length === 0) {
            els.peerList.innerHTML = `
                <div class="timeline-empty">
                    <span class="empty-icon">⬡</span>
                    <p>No peers discovered yet</p>
                </div>`;
            return;
        }

        els.peerList.innerHTML = '';
        peers.forEach(peer => {
            const card = document.createElement('div');
            card.className = 'peer-card';

            let badgeClass = 'ok';
            let badgeText = 'active';
            if (peer.banned) { badgeClass = 'banned'; badgeText = 'banned'; }
            else if (peer.backed_off) { badgeClass = 'backoff'; badgeText = 'backoff'; }

            const scoreHtml = peer.score > 0
                ? `<span class="peer-score">score: ${peer.score}</span>`
                : '';

            card.innerHTML = `
                <span class="peer-addr">${escapeHtml(peer.address)}</span>
                <div class="peer-info">
                    ${scoreHtml}
                    <span class="peer-badge ${badgeClass}">${badgeText}</span>
                </div>
            `;
            els.peerList.appendChild(card);
        });
    }

    // --- Connection Status ---
    function updateConnectionStatus(online) {
        els.statusPill.classList.toggle('online', online);
        els.statusPill.querySelector('.status-label').textContent = online ? 'online' : 'connecting';
    }

    // --- Utilities ---
    function escapeHtml(str) {
        const div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    }

    function relativeTime(unixSec) {
        const diff = Math.floor(Date.now() / 1000) - unixSec;
        if (diff < 60) return 'just now';
        if (diff < 3600) {
            const m = Math.floor(diff / 60);
            return m === 1 ? '1 min ago' : `${m} min ago`;
        }
        if (diff < 86400) {
            const h = Math.floor(diff / 3600);
            return h === 1 ? '1 hr ago' : `${h} hr ago`;
        }
        const d = Math.floor(diff / 86400);
        return d === 1 ? '1 day ago' : `${d} days ago`;
    }

    // --- Load More ---
    els.loadMoreBtn.addEventListener('click', loadMore);

    // --- Init ---
    async function init() {
        const data = await fetchTimeline(0, 50);
        const apiAvailable = !!data;

        if (apiAvailable) {
            timelineMessages = data.messages || [];
            timelineOffset = data.next_offset;
            hasMore = data.has_more;
            renderTimeline();
            updateConnectionStatus(true);
            connectSSE();
        } else {
            // Static preview mode — UI renders but no data
            updateConnectionStatus(false);
        }

        // Refresh when tabs are active
        setInterval(() => {
            if (currentTab === 'status') fetchStatus();
            if (currentTab === 'peers') fetchPeers();
        }, 10000);

        setInterval(() => fetchStatus(), 30000);
    }

    init();
})();
