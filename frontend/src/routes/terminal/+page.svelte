<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { backendUrl, pocketbase } from '$lib/stores/pocketbase';
	import type { Terminal } from '@xterm/xterm';
	import type { FitAddon } from '@xterm/addon-fit';
	import '@xterm/xterm/css/xterm.css';

	let host: HTMLDivElement;
	let pageElement: HTMLDivElement;
	let terminal: Terminal | undefined;
	let fit: FitAddon | undefined;
	let socket: WebSocket | undefined;
	let device = $state<{ id: string; name: string; ip: string }>();
	let username = $state('balint');
	let password = $state('');
	let connectionState = $state('Loading');
	let error = $state('');
	let showLogin = $state(false);
	let disposed = false;
	const keys = [ ['Esc', '\x1b'], ['Tab', '\t'], ['Ctrl+C', '\x03'], ['Ctrl+D', '\x04'], ['↑', '\x1b[A'], ['↓', '\x1b[B'], ['←', '\x1b[D'], ['→', '\x1b[C'] ];

	// Forward terminal input only to the currently authenticated SSH session.
	function sendInput(data: string) {
		if (connectionState === 'Connected' && socket?.readyState === WebSocket.OPEN) {
			socket.send(JSON.stringify({ type: 'input', data }));
		}
	}

	// Resize the page to the visual viewport in pixels, then fit and send the remote PTY size.
	// No arguments; returns early after disposal or before the renderer is ready.
	function resizeTerminal() {
		if (disposed || !pageElement) return;
		const viewport = window.visualViewport;
		const bottom = (viewport?.height ?? window.innerHeight) + (viewport?.offsetTop ?? 0);
		const top = Math.max(pageElement.getBoundingClientRect().top, viewport?.offsetTop ?? 0);
		const parent = pageElement.parentElement;
		const gutter = parent ? getComputedStyle(parent) : undefined;
		const bottomGutter = gutter ? parseFloat(gutter.paddingBottom) + parseFloat(gutter.marginBottom) : 0;
		pageElement.style.setProperty('--terminal-height', `${Math.max(180, Math.floor(bottom - top - bottomGutter))}px`);
		if (!terminal || !fit) return;
		fit.fit();
		if (connectionState === 'Connected' && socket?.readyState === WebSocket.OPEN) {
			socket.send(JSON.stringify({ type: 'resize', cols: terminal.cols, rows: terminal.rows }));
		}
	}

	// Closing the browser socket releases the corresponding server-owned SSH channel.
	function disconnect() {
		const previous = socket;
		socket = undefined;
		previous?.close();
		password = '';
		connectionState = 'Disconnected';
	}

	// SubmitEvent event cancels form navigation; sends credentials once over WSS and clears the password.
	// Returns without connecting when the device, renderer or credentials are unavailable.
	function connect(event: SubmitEvent) {
		event.preventDefault();
		if (!device || !terminal || !password || connectionState === 'Connecting' || connectionState === 'Connected') return;
		error = '';
		connectionState = 'Connecting';
		terminal.reset();
		const address = new URL(backendUrl + 'api/upsnap/terminal/connect', window.location.href);
		address.protocol = address.protocol === 'https:' ? 'wss:' : 'ws:';
		let secret = password;
		password = '';
		const ws = new WebSocket(address);
		socket = ws;
		ws.binaryType = 'arraybuffer';
		ws.onopen = () => {
			if (socket !== ws || disposed) { secret = ''; ws.close(); return; }
			ws.send(JSON.stringify({ type: 'auth', token: $pocketbase.authStore.token, device: device!.id,
				username, password: secret, cols: terminal!.cols, rows: terminal!.rows }));
			secret = '';
		};
		ws.onmessage = (message) => {
			if (socket !== ws || disposed) return;
			if (message.data instanceof ArrayBuffer) { terminal?.write(new Uint8Array(message.data)); return; }
			try {
				const frame = JSON.parse(message.data);
				if (frame.type === 'ready') { connectionState = 'Connected'; showLogin = false; resizeTerminal(); terminal?.focus(); }
				else if (frame.type === 'error') { error = frame.message; connectionState = 'Disconnected'; }
				else if (frame.type === 'exit') { connectionState = 'Disconnected'; }
			} catch { error = 'Invalid terminal response.'; }
		};
		ws.onerror = () => { if (socket === ws) error = 'Terminal connection failed.'; };
		ws.onclose = () => {
			secret = '';
			if (socket === ws) { socket = undefined; connectionState = 'Disconnected'; }
		};
	}

	// Mount with no arguments: load host metadata/renderers and observe viewport changes.
	// Returns cleanup that closes SSH and disposes listeners, observers and the renderer.
	onMount(function mountTerminal() {
		disposed = false;
		const unsubscribe = $pocketbase.authStore.onChange(() => {
			if (!$pocketbase.authStore.isValid) { disconnect(); void goto(resolve('/login')); }
		});
		const observer = new ResizeObserver(resizeTerminal);
		observer.observe(host);
		window.visualViewport?.addEventListener('resize', resizeTerminal);
		window.visualViewport?.addEventListener('scroll', resizeTerminal);
		window.addEventListener('resize', resizeTerminal);
		resizeTerminal();
		void (async () => {
			if (!$pocketbase.authStore.isValid) { await goto(resolve('/login')); return; }
			try {
				const id = new URL(window.location.href).searchParams.get('device') ?? '';
				const response = await fetch(backendUrl + 'api/upsnap/terminal/devices/' + encodeURIComponent(id),
					{ headers: { Authorization: $pocketbase.authStore.token } });
				if (!response.ok) throw new Error('Device SSH is unavailable or terminal access is not configured.');
				const reading = await response.json();
				const [{ Terminal }, { FitAddon }] = await Promise.all([import('@xterm/xterm'), import('@xterm/addon-fit')]);
				if (disposed) return;
				device = reading;
				username = reading.username || 'balint';
				terminal = new Terminal({ cursorBlink: true, fontSize: 14, scrollback: 3000,
					theme: { background: '#101923', foreground: '#e7eef5' } });
				fit = new FitAddon();
				terminal.loadAddon(fit);
				terminal.open(host);
				terminal.onData(sendInput);
				resizeTerminal();
				connectionState = 'Disconnected';
			} catch (cause) { if (!disposed) { error = String(cause instanceof Error ? cause.message : cause); connectionState = 'Unavailable'; } }
		})();
		return () => {
			disposed = true;
			disconnect();
			unsubscribe();
			observer.disconnect();
			window.visualViewport?.removeEventListener('resize', resizeTerminal);
			window.visualViewport?.removeEventListener('scroll', resizeTerminal);
			window.removeEventListener('resize', resizeTerminal);
			terminal?.dispose();
		};
	});
</script>

<div class="terminal-page" bind:this={pageElement}>
	<header><div><h1>Terminal{device ? ` · ${device.name}` : ''}</h1><span>{device?.ip ?? ''}</span></div><a class="btn btn-sm" href={resolve('/')}>Devices</a></header>
	<form onsubmit={connect} class:session-connected={connectionState === 'Connected' && !showLogin} aria-label="SSH connection">
		<label>SSH username<input class="input input-bordered" name="ssh_username" autocomplete="username" bind:value={username} required disabled={connectionState === 'Connected' || connectionState === 'Connecting'} /></label>
		<label>SSH password<input class="input input-bordered" type="password" name="ssh_password" autocomplete="current-password" bind:value={password} required disabled={connectionState === 'Connected' || connectionState === 'Connecting'} /></label>
		<div class="terminal-actions">
		<button class="btn connect-button" type="submit" disabled={!device || connectionState === 'Connected' || connectionState === 'Connecting'}>Connect</button>
		<button class="btn" type="button" onclick={disconnect} disabled={connectionState !== 'Connected' && connectionState !== 'Connecting'}>Disconnect</button>
		{#if connectionState === 'Connected'}<button class="btn terminal-settings" type="button" aria-expanded={showLogin} onclick={() => { showLogin = !showLogin; }}>Login</button>{/if}
		<span class="terminal-state" role="status">{connectionState}</span>
		</div>
	</form>
	{#if error}<p class="terminal-error" role="alert">{error}</p>{/if}
	<div class="terminal-host" bind:this={host}></div>
	<div class="terminal-keys" aria-label="Terminal keys">
		{#each keys as [label, code]}
			<button class="btn btn-sm" disabled={connectionState !== 'Connected'} onpointerdown={(event) => event.preventDefault()} onclick={() => { sendInput(code); terminal?.focus(); }}>{label}</button>
		{/each}
	</div>
</div>

<style>
	.terminal-page { max-width: 1500px; width: 100%; height: var(--terminal-height, calc(100dvh - 68px)); margin: auto; padding: 1rem 1.5rem; display: flex; flex-direction: column; gap: .75rem; min-width: 0; }
	header { display: flex; align-items: center; justify-content: space-between; gap: .75rem; flex-shrink: 0; }
	header > div { min-width: 0; }
	h1 { font-size: 1.2rem; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	header span, .terminal-state { font-size: .8rem; color: var(--monitor-muted); }
	header > a { flex-shrink: 0; }
	form { display: flex; align-items: end; gap: .75rem; flex-wrap: wrap; flex-shrink: 0; }
	label { display: grid; gap: .25rem; font-size: .8rem; min-width: 0; }
	input { width: 100%; min-width: 0; }
	.terminal-actions { display: flex; align-items: center; gap: .5rem; flex-wrap: wrap; }
	.terminal-settings { display: none; }
	.terminal-host { flex: 1; min-height: 64px; min-width: 0; background: #101923; padding: .5rem; border-radius: 8px; overflow: hidden; }
	.terminal-error { color: var(--color-error); font-size: .8rem; flex-shrink: 0; }
	.terminal-keys { display: grid; grid-template-columns: repeat(8, minmax(0, 1fr)); gap: .375rem; flex-shrink: 0; }
	.terminal-keys button { min-width: 0; padding-inline: .25rem; touch-action: manipulation; }
	@media (max-width: 768px), (max-height: 500px) {
		.terminal-page { padding: .5rem .75rem; gap: .5rem; }
		h1 { font-size: 1rem; }
		form { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: .5rem; }
		input { height: 40px; font-size: 16px; }
		.terminal-host :global(.xterm-helper-textarea) { font-size: 16px; }
		.terminal-actions { grid-column: 1 / -1; gap: .5rem; }
		.terminal-actions button { height: 36px; min-height: 36px; padding-inline: .65rem; }
		.terminal-state { margin-left: auto; font-size: .75rem; }
		.terminal-settings { display: inline-flex; }
		.session-connected label, .session-connected .connect-button { display: none; }
		.terminal-keys { grid-template-columns: repeat(4, minmax(0, 1fr)); }
		.terminal-keys button { height: 36px; min-height: 36px; }
	}
	@media (min-width: 601px) { .terminal-keys { grid-template-columns: repeat(8, minmax(0, 1fr)); } }
</style>
