<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { backendUrl, pocketbase } from '$lib/stores/pocketbase';
	import type { Terminal } from '@xterm/xterm';
	import type { FitAddon } from '@xterm/addon-fit';
	import '@xterm/xterm/css/xterm.css';

	let host: HTMLDivElement;
	let terminal: Terminal | undefined;
	let fit: FitAddon | undefined;
	let socket: WebSocket | undefined;
	let device = $state<{ id: string; name: string; ip: string }>();
	let username = $state('balint');
	let password = $state('');
	let connectionState = $state('Loading');
	let error = $state('');
	let disposed = false;
	const keys = [ ['Esc', '\x1b'], ['Tab', '\t'], ['Ctrl+C', '\x03'], ['Ctrl+D', '\x04'], ['↑', '\x1b[A'], ['↓', '\x1b[B'], ['←', '\x1b[D'], ['→', '\x1b[C'] ];

	// Forward terminal input only to the currently authenticated SSH session.
	function sendInput(data: string) {
		if (connectionState === 'Connected' && socket?.readyState === WebSocket.OPEN) {
			socket.send(JSON.stringify({ type: 'input', data }));
		}
	}

	// Fit both renderer and remote PTY to the visible terminal container.
	function resizeTerminal() {
		if (!terminal || !fit || disposed) return;
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

	// Send credentials once over same-origin WSS, then clear the password field and closure.
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
				if (frame.type === 'ready') { connectionState = 'Connected'; resizeTerminal(); terminal?.focus(); }
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

	// Load host metadata and terminal libraries in the browser; dispose all session resources on navigation.
	onMount(function mountTerminal() {
		disposed = false;
		const unsubscribe = $pocketbase.authStore.onChange(() => {
			if (!$pocketbase.authStore.isValid) { disconnect(); void goto(resolve('/login')); }
		});
		const observer = new ResizeObserver(resizeTerminal);
		observer.observe(host);
		window.visualViewport?.addEventListener('resize', resizeTerminal);
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
			terminal?.dispose();
		};
	});
</script>

<div class="terminal-page">
	<header><div><h1>Terminal{device ? ` · ${device.name}` : ''}</h1><span>{device?.ip ?? ''}</span></div><a class="btn btn-sm" href={resolve('/')}>Devices</a></header>
	<form onsubmit={connect}>
		<label>SSH username<input class="input input-bordered" name="ssh_username" autocomplete="username" bind:value={username} required disabled={connectionState === 'Connected' || connectionState === 'Connecting'} /></label>
		<label>SSH password<input class="input input-bordered" type="password" name="ssh_password" autocomplete="current-password" bind:value={password} required disabled={connectionState === 'Connected' || connectionState === 'Connecting'} /></label>
		<button class="btn" type="submit" disabled={!device || connectionState === 'Connected' || connectionState === 'Connecting'}>Connect</button>
		<button class="btn" type="button" onclick={disconnect} disabled={connectionState !== 'Connected' && connectionState !== 'Connecting'}>Disconnect</button>
		<span class="terminal-state" role="status">{connectionState}</span>
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
	.terminal-page { max-width: 1500px; margin: auto; padding: 1.5rem; }
	header { display: flex; align-items: center; justify-content: space-between; gap: 1rem; margin-bottom: 1rem; }
	h1 { font-size: 1.3rem; font-weight: 600; }
	header span, .terminal-state { font-size: .8rem; color: var(--monitor-muted); }
	form { display: flex; align-items: end; gap: .75rem; flex-wrap: wrap; margin-bottom: 1rem; }
	label { display: grid; gap: .3rem; font-size: .8rem; }
	.terminal-state { padding-block: .75rem; }
	.terminal-host { height: 65dvh; min-height: 280px; background: #101923; padding: .7rem; border-radius: 8px; overflow: hidden; }
	.terminal-error { color: var(--color-error); margin-bottom: .75rem; }
	.terminal-keys { display: flex; gap: .4rem; overflow-x: auto; padding-block: .75rem; }
	@media (max-width: 600px) { .terminal-page { padding: .75rem; } label { width: 100%; } input { width: 100%; } h1 { font-size: 1rem; } }
</style>
