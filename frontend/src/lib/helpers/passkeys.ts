import { backendUrl } from '$lib/stores/pocketbase';
import type PocketBase from 'pocketbase';

export async function passkeyRequest(path: string, token = '', method = 'GET', body?: unknown) {
	const response = await fetch(`${backendUrl}api/upsnap/passkeys${path}`, {
		method,
		headers: {
			...(token ? { Authorization: token } : {}),
			...(body ? { 'Content-Type': 'application/json' } : {})
		},
		body: body ? JSON.stringify(body) : undefined,
		cache: 'no-store'
	});
	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(error.message || 'Passkey request failed');
	}
	return response.status === 204 ? null : response.json();
}
function decode(value: string): ArrayBuffer {
	const text = atob(value.replace(/-/g, '+').replace(/_/g, '/'));
	return Uint8Array.from(text, (c) => c.charCodeAt(0)).buffer;
}
function encode(value: ArrayBuffer | null): string | null {
	return value === null
		? null
		: btoa(String.fromCharCode(...new Uint8Array(value)))
				.replace(/\+/g, '-')
				.replace(/\//g, '_')
				.replace(/=+$/, '');
}
function serialize(credential: PublicKeyCredential) {
	const response = credential.response;
	const common = {
		id: credential.id,
		rawId: encode(credential.rawId),
		type: credential.type,
		clientExtensionResults: credential.getClientExtensionResults(),
		authenticatorAttachment: credential.authenticatorAttachment
	};
	if (response instanceof AuthenticatorAttestationResponse)
		return {
			...common,
			response: {
				clientDataJSON: encode(response.clientDataJSON),
				attestationObject: encode(response.attestationObject),
				transports: response.getTransports?.() ?? []
			}
		};
	const assertion = response as AuthenticatorAssertionResponse;
	return {
		...common,
		response: {
			clientDataJSON: encode(assertion.clientDataJSON),
			authenticatorData: encode(assertion.authenticatorData),
			signature: encode(assertion.signature),
			userHandle: encode(assertion.userHandle)
		}
	};
}
export async function enrollPasskey(pb: PocketBase, label: string) {
	const start = await passkeyRequest('/register/begin', pb.authStore.token, 'POST');
	const options = start.options.publicKey;
	options.challenge = decode(options.challenge);
	options.user.id = decode(options.user.id);
	options.excludeCredentials = (options.excludeCredentials ?? []).map((c: { id: string }) => ({
		...c,
		id: decode(c.id)
	}));
	const credential = (await navigator.credentials.create({
		publicKey: options
	})) as PublicKeyCredential | null;
	if (!credential) throw new Error('Passkey enrollment canceled');
	await passkeyRequest(
		`/register/finish?ceremony=${encodeURIComponent(start.ceremony)}&label=${encodeURIComponent(label)}`,
		pb.authStore.token,
		'POST',
		serialize(credential)
	);
}
export async function signInPasskey(pb: PocketBase) {
	const start = await passkeyRequest('/login/begin', '', 'POST');
	const options = start.options.publicKey;
	options.challenge = decode(options.challenge);
	if (options.allowCredentials)
		options.allowCredentials = options.allowCredentials.map((c: { id: string }) => ({
			...c,
			id: decode(c.id)
		}));
	const credential = (await navigator.credentials.get({
		publicKey: options
	})) as PublicKeyCredential | null;
	if (!credential) throw new Error('Passkey sign-in canceled');
	const result = await passkeyRequest(
		`/login/finish?ceremony=${encodeURIComponent(start.ceremony)}`,
		'',
		'POST',
		serialize(credential)
	);
	pb.authStore.save(result.token, result.record);
}
export function passkeyError(error: unknown): string {
	return error instanceof DOMException && error.name === 'NotAllowedError'
		? 'Passkey request canceled or unavailable. You can still sign in with your password.'
		: error instanceof Error
			? error.message
			: 'Passkey request failed';
}
