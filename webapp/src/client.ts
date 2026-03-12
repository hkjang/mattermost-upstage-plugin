import manifest from 'manifest';

let siteURL = '';

type RequestOptions = Omit<RequestInit, 'headers'> & {
    headers?: Record<string, string>;
};

export type BotDefinition = {
    id: string;
    username: string;
    display_name: string;
    description?: string;
    base_url?: string;
    auth_mode?: string;
    auth_token?: string;
    model?: string;
    mode?: string;
    ocr?: string;
    output_formats?: string[];
    coordinates?: boolean;
    chart_recognition?: boolean;
    merge_multipage_tables?: boolean;
    base64_encoding?: string[];
    allowed_teams?: string[];
    allowed_channels?: string[];
    allowed_users?: string[];
};

export type ExecutionRecord = {
    correlation_id: string;
    bot_id: string;
    bot_username: string;
    bot_name: string;
    model: string;
    status: string;
    error_message?: string;
    error_code?: string;
    source: string;
};

export type BotRunResult = {
    correlation_id: string;
    bot_id: string;
    bot_username: string;
    bot_name: string;
    model: string;
    api_duration_ms?: number;
    post_id?: string;
    status: string;
    output?: string;
    error_message?: string;
    error_code?: string;
    error_detail?: string;
    error_hint?: string;
    request_url?: string;
    http_status?: number;
    retryable?: boolean;
};

export type PluginStatus = {
    plugin_id: string;
    base_url: string;
    bot_count: number;
    allow_hosts: string[];
    bots: BotDefinition[];
    managed_bots: ManagedBotStatus[];
    bot_sync: BotSyncState;
    config_error?: string;
};

export type AdminPluginConfig = {
    service: {
        base_url: string;
        auth_mode: string;
        auth_token: string;
        allow_hosts: string;
    };
    runtime: {
        default_timeout_seconds: number;
        max_input_length: number;
        max_output_length: number;
        enable_debug_logs: boolean;
        enable_usage_logs: boolean;
    };
    bots: BotDefinition[];
};

export type AdminConfigResponse = {
    config: AdminPluginConfig;
    source: string;
};

export type ManagedBotStatus = {
    bot_id: string;
    username: string;
    display_name: string;
    model: string;
    user_id?: string;
    registered: boolean;
    active: boolean;
    status_message?: string;
};

export type BotSyncState = {
    last_error?: string;
    updated_at: number;
    entries: ManagedBotStatus[];
};

export type ConnectionStatus = {
    ok: boolean;
    url: string;
    status_code: number;
    message: string;
    error_code?: string;
    detail?: string;
    hint?: string;
    retryable?: boolean;
};

export function setSiteURL(value: string) {
    siteURL = value.replace(/\/+$/, '');
}

function pluginURL(path: string) {
    const base = siteURL || window.location.origin;
    return `${base}/plugins/${manifest.id}/api/v1${path}`;
}

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
    const response = await fetch(pluginURL(path), {
        ...options,
        headers: {
            'Content-Type': 'application/json',
            ...(options.headers || {}),
        },
    });

    const data = await response.json().catch(() => ({}));
    if (!response.ok) {
        const failure = data as {error?: string; error_message?: string};
        throw new Error(failure.error || failure.error_message || 'Request failed');
    }
    return data as T;
}

export async function getStatus() {
    return request<PluginStatus>('/status');
}

export async function getAdminConfig() {
    return request<AdminConfigResponse>('/config');
}

export async function testConnection() {
    return request<ConnectionStatus>('/test', {method: 'POST'});
}

export async function getBots(channelId?: string) {
    const query = channelId ? `?channel_id=${encodeURIComponent(channelId)}` : '';
    const response = await request<{bots: BotDefinition[]}>(`/bots${query}`);
    return response.bots;
}

export async function getHistory(limit = 5) {
    const response = await request<{items: ExecutionRecord[]}>(`/history?limit=${limit}`);
    return response.items;
}

export async function runBot(payload: {
    bot_id: string;
    channel_id: string;
    root_id?: string;
    prompt: string;
    file_ids?: string[];
}) {
    return request<BotRunResult>('/run', {
        method: 'POST',
        body: JSON.stringify(payload),
    });
}
