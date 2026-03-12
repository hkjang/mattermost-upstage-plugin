import manifest from 'manifest';
import React, {useEffect, useMemo, useRef, useState} from 'react';

import type {AdminPluginConfig, BotDefinition, ConnectionStatus, PluginStatus} from '../client';
import {getAdminConfig, getStatus, testConnection} from '../client';

const defaultURL = 'https://api.upstage.ai/v1/document-digitization';
const defaultModel = 'document-parse';
const defaultFormats: string[] = [];

type DraftBot = {
    local_id: string;
    username: string;
    display_name: string;
    description: string;
    base_url: string;
    auth_mode: string;
    auth_token: string;
    model: string;
    mode: string;
    ocr: string;
    output_formats: string[];
    coordinates: boolean;
    chart_recognition: boolean;
    merge_multipage_tables: boolean;
    base64_encoding: string[];
    allowed_teams: string[];
    allowed_channels: string[];
    allowed_users: string[];
};

type DraftConfig = {
    service: {base_url: string; auth_mode: string; auth_token: string; allow_hosts: string};
    runtime: {default_timeout_seconds: number; max_input_length: number; max_output_length: number; enable_debug_logs: boolean; enable_usage_logs: boolean};
    bots: DraftBot[];
};

type Props = {
    id?: string;
    value?: unknown;
    disabled?: boolean;
    setByEnv?: boolean;
    helpText?: React.ReactNode;
    onChange: (id: string, value: unknown) => void;
    setSaveNeeded?: () => void;
};

const stack: React.CSSProperties = {display: 'flex', flexDirection: 'column', gap: 16};
const card: React.CSSProperties = {background: 'white', border: '1px solid rgba(63,67,80,.12)', borderRadius: 8, padding: 20, display: 'flex', flexDirection: 'column', gap: 12};
const row2: React.CSSProperties = {display: 'grid', gridTemplateColumns: 'repeat(2,minmax(0,1fr))', gap: 12};
const row3: React.CSSProperties = {display: 'grid', gridTemplateColumns: 'repeat(3,minmax(0,1fr))', gap: 12};
const botLayout: React.CSSProperties = {display: 'grid', gridTemplateColumns: '300px minmax(0,1fr)', gap: 16};
const field: React.CSSProperties = {width: '100%', border: '1px solid rgba(63,67,80,.16)', borderRadius: 8, padding: '10px 12px'};
const note: React.CSSProperties = {fontSize: 12, opacity: 0.75};
const box: React.CSSProperties = {padding: 12, borderRadius: 8, background: 'rgba(var(--button-bg-rgb),.08)', border: '1px solid rgba(var(--button-bg-rgb),.18)'};

const sampleBots: Partial<BotDefinition>[] = [
    {username: 'upstage-parser', display_name: '문서 파서 기본', description: '기본 파서 봇', model: defaultModel, mode: 'standard', ocr: 'auto', output_formats: [], coordinates: true, chart_recognition: true, merge_multipage_tables: false},
    {username: 'upstage-html', display_name: '문서 파서 HTML', description: 'HTML 출력 강화 봇', model: defaultModel, mode: 'enhanced', ocr: 'force', output_formats: ['html', 'markdown', 'text'], coordinates: true, chart_recognition: true, merge_multipage_tables: true},
];

export default function ConfigSetting(props: Props) {
    const key = props.id || 'Config';
    const disabled = Boolean(props.disabled || props.setByEnv);
    const [config, setConfig] = useState<DraftConfig>(createDefaultConfig());
    const [selected, setSelected] = useState('');
    const [status, setStatus] = useState<PluginStatus | null>(null);
    const [connection, setConnection] = useState<ConnectionStatus | null>(null);
    const [source, setSource] = useState('config');
    const [error, setError] = useState('');
    const [loadingConfig, setLoadingConfig] = useState(true);
    const [loadingStatus, setLoadingStatus] = useState(true);
    const [testing, setTesting] = useState(false);
    const last = useRef('');

    useEffect(() => { void loadConfig(props.value, last, setConfig, setSource, setSelected, setLoadingConfig, setError); }, [props.value]);
    useEffect(() => { void loadStatus(setStatus, setLoadingStatus, setError); }, []);

    const bot = useMemo(() => config.bots.find((item) => item.local_id === selected) || config.bots[0] || null, [config.bots, selected]);
    const messages = useMemo(() => validate(config), [config]);

    const apply = (next: DraftConfig, nextSelected?: string) => {
        setConfig(next);
        const raw = JSON.stringify(buildConfig(next), null, 2);
        last.current = raw;
        props.onChange(key, raw);
        props.setSaveNeeded?.();
        setSelected(nextSelected || pickBot(next.bots, selected));
    };

    return <div style={stack}>{renderPlaceholder({bot, card, messages, error, loadingConfig, source, props, manifest, status, loadingStatus, connection, testing, config, disabled, apply, setConnection, setTesting, setError, setSelected})}</div>;
}

function renderPlaceholder(args: any) {
    const {bot, card, messages, error, loadingConfig, source, props, manifest, status, loadingStatus, connection, testing, config, disabled, apply, setConnection, setTesting, setError, setSelected} = args;
    const updateService = (patch: Partial<DraftConfig['service']>) => apply({...config, service: {...config.service, ...patch}});
    const updateRuntime = (patch: Partial<DraftConfig['runtime']>) => apply({...config, runtime: {...config.runtime, ...patch}});
    const updateBot = (id: string, patch: Partial<DraftBot>) => apply({...config, bots: config.bots.map((item: DraftBot) => item.local_id === id ? {...item, ...patch} : item)}, id);
    const test = async () => {
        setTesting(true);
        setConnection(null);
        setError('');
        try {
            setConnection(await testConnection());
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setTesting(false);
        }
    };

    return <>
        <section style={card}>
            <div style={{display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'center'}}>
                <strong>{'Upstage Document Parser 설정'}</strong>
                <span style={{fontSize: 12, fontWeight: 700}}>{manifest.version}</span>
            </div>
            <span style={note}>{'여러 Mattermost 봇에 서로 다른 Upstage 문서 파서 입력 파라미터를 지정할 수 있습니다.'}</span>
            <div style={box}>
                <div>{'봇은 DM 또는 @멘션 + 파일 첨부로 호출됩니다.'}</div>
                <div>{'메시지 본문은 Upstage API로 전달되지 않습니다. 실제 요청은 첨부 파일만 document 파트로 업로드합니다.'}</div>
                <div>{'base64_encoding은 업로드 이미지 자체가 아니라, 응답에 base64로 포함할 레이아웃 카테고리(table 등)를 지정하는 옵션입니다.'}</div>
                <div>{'output_formats를 비워 두면 Upstage API 기본 출력 형식을 사용합니다.'}</div>
            </div>
            {source === 'legacy' && <div style={box}>{'기존 개별 설정을 불러왔습니다. 저장하면 단일 Config 형식으로 정리됩니다.'}</div>}
            {props.setByEnv && <div style={box}>{'이 설정은 환경 변수로 관리되고 있어 여기에서 수정할 수 없습니다.'}</div>}
            {props.helpText}
            {error && <div style={box}>{error}</div>}
            {messages.length > 0 && <div style={box}>{messages.map((m: string) => <div key={m}>{m}</div>)}</div>}
        </section>

        <section style={card}>
            <strong>{'서비스 연결'}</strong>
            {loadingConfig ? <span>{'설정을 불러오는 중입니다...'}</span> : <>
                <div style={row2}>
                    <Field label={'기본 URL'}><input disabled={disabled} style={field} value={config.service.base_url} placeholder={defaultURL} onChange={(e) => updateService({base_url: e.target.value})}/></Field>
                    <Field label={'인증 방식'}>
                        <select disabled={disabled} style={field} value={config.service.auth_mode} onChange={(e) => updateService({auth_mode: e.target.value})}>
                            <option value='bearer'>{'Authorization: Bearer'}</option>
                            <option value='x-api-key'>{'x-api-key'}</option>
                        </select>
                    </Field>
                </div>
                <div style={row2}>
                    <Field label={'기본 API 키'}><input disabled={disabled} type='password' style={field} value={config.service.auth_token} onChange={(e) => updateService({auth_token: e.target.value})}/></Field>
                    <Field label={'허용 호스트'}><input disabled={disabled} style={field} value={config.service.allow_hosts} placeholder={'api.upstage.ai'} onChange={(e) => updateService({allow_hosts: e.target.value})}/></Field>
                </div>
                <div style={row3}>
                    <Field label={'타임아웃(초)'}><input disabled={disabled} type='number' min={1} style={field} value={String(config.runtime.default_timeout_seconds)} onChange={(e) => updateRuntime({default_timeout_seconds: num(e.target.value, 30)})}/></Field>
                    <Field label={'최대 메시지 길이'}><input disabled={disabled} type='number' min={1} style={field} value={String(config.runtime.max_input_length)} onChange={(e) => updateRuntime({max_input_length: num(e.target.value, 4000)})}/></Field>
                    <Field label={'최대 응답 길이'}><input disabled={disabled} type='number' min={1} style={field} value={String(config.runtime.max_output_length)} onChange={(e) => updateRuntime({max_output_length: num(e.target.value, 8000)})}/></Field>
                </div>
                <label><input disabled={disabled} type='checkbox' checked={config.runtime.enable_debug_logs} onChange={(e) => updateRuntime({enable_debug_logs: e.target.checked})}/>{' 디버그 로그'}</label>
                <label><input disabled={disabled} type='checkbox' checked={config.runtime.enable_usage_logs} onChange={(e) => updateRuntime({enable_usage_logs: e.target.checked})}/>{' 사용량 로그'}</label>
            </>}
        </section>

        <section style={card}>
            <div style={{display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'center'}}>
                <strong>{'봇 카탈로그'}</strong>
                <div style={{display: 'flex', gap: 8}}>
                    <button className='btn btn-tertiary' disabled={disabled} type='button' onClick={() => apply({...config, bots: sampleBots.map((item, i) => normalizeBot(item, i))}, 'bot-0')}>{'예시 불러오기'}</button>
                    <button className='btn btn-primary' disabled={disabled} type='button' onClick={() => { const next = emptyBot(); apply({...config, bots: [...config.bots, next]}, next.local_id); }}>{'봇 추가'}</button>
                </div>
            </div>
            <div style={botLayout}>
                <div style={{display: 'flex', flexDirection: 'column', gap: 8}}>
                    {config.bots.length === 0 && <div style={box}>{'아직 등록된 봇이 없습니다.'}</div>}
                    {config.bots.map((item: DraftBot) => <button key={item.local_id} type='button' onClick={() => setSelected(item.local_id)} style={{...box, textAlign: 'left', borderColor: bot?.local_id === item.local_id ? 'rgba(var(--button-bg-rgb),.5)' : 'transparent'}}><strong>{item.display_name || '@new-bot'}</strong><div>{`@${item.username || 'username'}`}</div><div style={note}>{`${item.model} | ${item.mode}`}</div></button>)}
                </div>
                <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
                    {!bot && <div style={box}>{'왼쪽에서 봇을 선택하세요.'}</div>}
                    {bot && <>
                        <div style={{display: 'flex', justifyContent: 'space-between', gap: 12}}>
                            <strong>{bot.display_name || '@new-bot'}</strong>
                            <div style={{display: 'flex', gap: 8}}>
                                <button className='btn btn-tertiary' disabled={disabled} type='button' onClick={() => { const copy = {...bot, local_id: id('bot'), username: bot.username ? `${bot.username}-copy` : '', display_name: bot.display_name ? `${bot.display_name} 복사본` : '', output_formats: [...bot.output_formats], base64_encoding: [...bot.base64_encoding], allowed_teams: [...bot.allowed_teams], allowed_channels: [...bot.allowed_channels], allowed_users: [...bot.allowed_users]}; apply({...config, bots: [...config.bots, copy]}, copy.local_id); }}>{'복제'}</button>
                                <button className='btn btn-danger' disabled={disabled} type='button' onClick={() => apply({...config, bots: config.bots.filter((item: DraftBot) => item.local_id !== bot.local_id)})}>{'삭제'}</button>
                            </div>
                        </div>
                        <div style={row2}>
                            <Field label={'username'}><input disabled={disabled} style={field} value={bot.username} placeholder={'upstage-parser'} onChange={(e) => updateBot(bot.local_id, {username: user(e.target.value)})}/></Field>
                            <Field label={'표시 이름'}><input disabled={disabled} style={field} value={bot.display_name} onChange={(e) => updateBot(bot.local_id, {display_name: e.target.value})}/></Field>
                        </div>
                        <Field label={'설명'}><textarea disabled={disabled} style={{...field, minHeight: 72}} value={bot.description} onChange={(e) => updateBot(bot.local_id, {description: e.target.value})}/></Field>
                        <div style={row3}>
                            <Field label={'model'}><input disabled={disabled} style={field} value={bot.model} onChange={(e) => updateBot(bot.local_id, {model: e.target.value || defaultModel})}/></Field>
                            <Field label={'mode'}><select disabled={disabled} style={field} value={bot.mode} onChange={(e) => updateBot(bot.local_id, {mode: mode(e.target.value)})}><option value='standard'>{'standard'}</option><option value='enhanced'>{'enhanced'}</option><option value='auto'>{'auto'}</option></select></Field>
                            <Field label={'ocr'}><select disabled={disabled} style={field} value={bot.ocr} onChange={(e) => updateBot(bot.local_id, {ocr: ocr(e.target.value)})}><option value='auto'>{'auto'}</option><option value='force'>{'force'}</option></select></Field>
                        </div>
                        <div style={row2}>
                            <Field label={'output_formats'}><input disabled={disabled} style={field} value={join(bot.output_formats)} placeholder={'비워 두면 API 기본값 사용'} onChange={(e) => updateBot(bot.local_id, {output_formats: formats(split(e.target.value, true))})}/></Field>
                            <Field label={'base64_encoding'}><input disabled={disabled} style={field} value={join(bot.base64_encoding)} placeholder={'table'} onChange={(e) => updateBot(bot.local_id, {base64_encoding: split(e.target.value, true)})}/></Field>
                        </div>
                        <div style={row2}>
                            <Field label={'봇 전용 URL'}><input disabled={disabled} style={field} value={bot.base_url} placeholder={'비워 두면 기본 URL 사용'} onChange={(e) => updateBot(bot.local_id, {base_url: e.target.value})}/></Field>
                            <Field label={'봇 전용 API 키'}><input disabled={disabled} type='password' style={field} value={bot.auth_token} placeholder={'비워 두면 기본 키 사용'} onChange={(e) => updateBot(bot.local_id, {auth_token: e.target.value})}/></Field>
                        </div>
                        <Field label={'봇 전용 인증 방식'}><select disabled={disabled} style={field} value={bot.auth_mode} onChange={(e) => updateBot(bot.local_id, {auth_mode: botAuth(e.target.value)})}><option value=''>{'기본값 사용'}</option><option value='bearer'>{'Authorization: Bearer'}</option><option value='x-api-key'>{'x-api-key'}</option></select></Field>
                        <label><input disabled={disabled} type='checkbox' checked={bot.coordinates} onChange={(e) => updateBot(bot.local_id, {coordinates: e.target.checked})}/>{' coordinates'}</label>
                        <label><input disabled={disabled} type='checkbox' checked={bot.chart_recognition} onChange={(e) => updateBot(bot.local_id, {chart_recognition: e.target.checked})}/>{' chart_recognition'}</label>
                        <label><input disabled={disabled} type='checkbox' checked={bot.merge_multipage_tables} onChange={(e) => updateBot(bot.local_id, {merge_multipage_tables: e.target.checked})}/>{' merge_multipage_tables'}</label>
                        <div style={row3}>
                            <Field label={'허용 팀'}><input disabled={disabled} style={field} value={join(bot.allowed_teams)} placeholder={'engineering'} onChange={(e) => updateBot(bot.local_id, {allowed_teams: split(e.target.value, true)})}/></Field>
                            <Field label={'허용 채널'}><input disabled={disabled} style={field} value={join(bot.allowed_channels)} placeholder={'town-square'} onChange={(e) => updateBot(bot.local_id, {allowed_channels: split(e.target.value, true)})}/></Field>
                            <Field label={'허용 사용자'}><input disabled={disabled} style={field} value={join(bot.allowed_users)} placeholder={'alice'} onChange={(e) => updateBot(bot.local_id, {allowed_users: split(e.target.value, true)})}/></Field>
                        </div>
                        <pre style={{...box, whiteSpace: 'pre-wrap', fontSize: 12}}>{curl(config, bot)}</pre>
                    </>}
                </div>
            </div>
        </section>

        <section style={card}>
            <strong>{'현재 상태'}</strong>
            {loadingStatus ? <span>{'플러그인 상태를 불러오는 중입니다...'}</span> : <>
                {status && <div style={box}><div>{`기본 URL: ${status.base_url || '설정되지 않음'}`}</div><div>{`봇 수: ${status.bot_count}`}</div><div>{`허용 호스트: ${(status.allow_hosts || []).join(', ') || '기본 URL 호스트 사용'}`}</div>{status.config_error && <div>{`설정 오류: ${status.config_error}`}</div>}{status.bot_sync?.last_error && <div>{`동기화 오류: ${status.bot_sync.last_error}`}</div>}</div>}
                <button className='btn btn-primary' disabled={testing} type='button' onClick={test}>{testing ? '연결 확인 중...' : '연결 테스트'}</button>
                {connection && <div style={box}><div>{connection.ok ? '연결에 성공했습니다.' : '연결에 실패했습니다.'}</div><div>{connection.url}</div><div style={{whiteSpace: 'pre-wrap'}}>{connection.message}</div>{connection.error_code && <div>{`오류 코드: ${connection.error_code}`}</div>}{connection.detail && <div style={{whiteSpace: 'pre-wrap'}}>{connection.detail}</div>}</div>}
            </>}
        </section>

        <details style={card}><summary>{'JSON 미리보기'}</summary><pre style={{...box, whiteSpace: 'pre-wrap', fontSize: 12}}>{JSON.stringify(buildConfig(config), null, 2)}</pre></details>
    </>;
}

function Field(props: {label: string; children: React.ReactNode}) {
    return <label style={{display: 'flex', flexDirection: 'column', gap: 6}}><strong>{props.label}</strong>{props.children}</label>;
}

function createDefaultConfig(): DraftConfig {
    return {
        service: {base_url: defaultURL, auth_mode: 'bearer', auth_token: '', allow_hosts: ''},
        runtime: {default_timeout_seconds: 30, max_input_length: 4000, max_output_length: 8000, enable_debug_logs: false, enable_usage_logs: true},
        bots: [],
    };
}

async function loadConfig(value: unknown, last: React.MutableRefObject<string>, setConfig: (v: DraftConfig) => void, setSource: (v: string) => void, setSelected: (v: string | ((v: string) => string)) => void, setLoading: (v: boolean) => void, setError: (v: string) => void) {
    setLoading(true);
    setError('');
    const raw = serialize(value);
    if (raw && raw === last.current) {
        setLoading(false);
        return;
    }
    const parsed = parseValue(value);
    if (parsed.ok) {
        setConfig(parsed.config);
        setSource('config');
        setSelected((current) => pickBot(parsed.config.bots, current));
        last.current = raw;
        setLoading(false);
        return;
    }
    try {
        const response = await getAdminConfig();
        const next = normalizeConfig(response.config);
        setConfig(next);
        setSource(response.source || 'config');
        setSelected((current) => pickBot(next.bots, current));
        last.current = serialize(buildConfig(next));
    } catch (e) {
        setError((e as Error).message);
    } finally {
        setLoading(false);
    }
}

async function loadStatus(setStatus: (v: PluginStatus | null) => void, setLoading: (v: boolean) => void, setError: (v: string) => void) {
    setLoading(true);
    try {
        setStatus(await getStatus());
    } catch (e) {
        setError((e as Error).message);
    } finally {
        setLoading(false);
    }
}

function parseValue(value: unknown) {
    if (value == null || value === '') {
        return {ok: false, config: createDefaultConfig()};
    }
    try {
        return {ok: true, config: normalizeConfig((typeof value === 'string' ? JSON.parse(value) : value) as AdminPluginConfig)};
    } catch {
        return {ok: false, config: createDefaultConfig()};
    }
}

function normalizeConfig(value?: AdminPluginConfig): DraftConfig {
    const next = createDefaultConfig();
    if (!value) {
        return next;
    }
    next.service = {
        base_url: text(value.service?.base_url) || defaultURL,
        auth_mode: auth(text(value.service?.auth_mode)),
        auth_token: text(value.service?.auth_token),
        allow_hosts: text(value.service?.allow_hosts),
    };
    next.runtime = {
        default_timeout_seconds: num(value.runtime?.default_timeout_seconds, 30),
        max_input_length: num(value.runtime?.max_input_length, 4000),
        max_output_length: num(value.runtime?.max_output_length, 8000),
        enable_debug_logs: Boolean(value.runtime?.enable_debug_logs),
        enable_usage_logs: value.runtime?.enable_usage_logs ?? true,
    };
    next.bots = Array.isArray(value.bots) ? value.bots.map((item, i) => normalizeBot(item, i)) : [];
    return next;
}

function buildConfig(config: DraftConfig): AdminPluginConfig {
    return {
        service: {base_url: config.service.base_url.trim(), auth_mode: auth(config.service.auth_mode), auth_token: config.service.auth_token.trim(), allow_hosts: config.service.allow_hosts.trim()},
        runtime: {default_timeout_seconds: num(config.runtime.default_timeout_seconds, 30), max_input_length: num(config.runtime.max_input_length, 4000), max_output_length: num(config.runtime.max_output_length, 8000), enable_debug_logs: Boolean(config.runtime.enable_debug_logs), enable_usage_logs: Boolean(config.runtime.enable_usage_logs)},
        bots: config.bots.map((item) => ({id: item.username.trim(), username: item.username.trim(), display_name: item.display_name.trim(), description: item.description.trim(), base_url: item.base_url.trim(), auth_mode: botAuth(item.auth_mode), auth_token: item.auth_token.trim(), model: item.model.trim() || defaultModel, mode: mode(item.mode), ocr: ocr(item.ocr), output_formats: formats(item.output_formats), coordinates: Boolean(item.coordinates), chart_recognition: Boolean(item.chart_recognition), merge_multipage_tables: Boolean(item.merge_multipage_tables), base64_encoding: split(join(item.base64_encoding), true), allowed_teams: split(join(item.allowed_teams), true), allowed_channels: split(join(item.allowed_channels), true), allowed_users: split(join(item.allowed_users), true)})),
    };
}

function normalizeBot(value: Partial<BotDefinition>, index = 0): DraftBot {
    return {
        local_id: `bot-${index}`,
        username: user(text(value.username)),
        display_name: text(value.display_name),
        description: text(value.description),
        base_url: text(value.base_url),
        auth_mode: botAuth(text(value.auth_mode)),
        auth_token: text(value.auth_token),
        model: text(value.model) || defaultModel,
        mode: mode(text(value.mode)),
        ocr: ocr(text(value.ocr)),
        output_formats: formats(value.output_formats),
        coordinates: value.coordinates ?? true,
        chart_recognition: value.chart_recognition ?? true,
        merge_multipage_tables: value.merge_multipage_tables ?? false,
        base64_encoding: split(join(Array.isArray(value.base64_encoding) ? value.base64_encoding : []), true),
        allowed_teams: split(join(Array.isArray(value.allowed_teams) ? value.allowed_teams : []), true),
        allowed_channels: split(join(Array.isArray(value.allowed_channels) ? value.allowed_channels : []), true),
        allowed_users: split(join(Array.isArray(value.allowed_users) ? value.allowed_users : []), true),
    };
}

function validate(config: DraftConfig) {
    const items: string[] = [];
    const names = new Set<string>();
    if (!config.service.base_url.trim()) {
        items.push('기본 URL은 필수입니다.');
    }
    config.bots.forEach((bot, i) => {
        const label = bot.display_name || bot.username || `봇 ${i + 1}`;
        if (!bot.username.trim()) {
            items.push(`${label}: username은 필수입니다.`);
        } else if (names.has(bot.username.trim())) {
            items.push(`${label}: username이 중복되었습니다.`);
        } else {
            names.add(bot.username.trim());
        }
        if (!bot.display_name.trim()) {
            items.push(`${label}: 표시 이름은 필수입니다.`);
        }
    });
    return items;
}

function curl(config: DraftConfig, bot: DraftBot) {
    const authLine = (bot.auth_mode || config.service.auth_mode) === 'x-api-key' ? '-H "x-api-key: $UPSTAGE_API_KEY"' : '-H "Authorization: Bearer $UPSTAGE_API_KEY"';
    const lines = [`curl -X POST "${bot.base_url || config.service.base_url || defaultURL}"`, authLine, '-H "Accept: application/json"', `-F "model=${bot.model || defaultModel}"`, '-F "document=@example.pdf"'];
    if (mode(bot.mode) !== 'standard') {
        lines.push(`-F "mode=${mode(bot.mode)}"`);
    }
    if (ocr(bot.ocr) !== 'auto') {
        lines.push(`-F "ocr=${ocr(bot.ocr)}"`);
    }
    if (formats(bot.output_formats).length > 0) {
        lines.push(`-F "output_formats=${JSON.stringify(formats(bot.output_formats))}"`);
    }
    if (!bot.coordinates) {
        lines.push(`-F "coordinates=${String(bot.coordinates)}"`);
    }
    if (!bot.chart_recognition) {
        lines.push(`-F "chart_recognition=${String(bot.chart_recognition)}"`);
    }
    if (bot.merge_multipage_tables) {
        lines.push(`-F "merge_multipage_tables=${String(bot.merge_multipage_tables)}"`);
    }
    return lines.map((line, index) => {
        const prefix = index === 0 ? '' : '  ';
        return index < lines.length - 1 ? `${prefix}${line} \\` : `${prefix}${line}`;
    }).join('\n');
}

function emptyBot(): DraftBot {
    return {local_id: id('bot'), username: '', display_name: '', description: '', base_url: '', auth_mode: '', auth_token: '', model: defaultModel, mode: 'standard', ocr: 'auto', output_formats: [...defaultFormats], coordinates: true, chart_recognition: true, merge_multipage_tables: false, base64_encoding: [], allowed_teams: [], allowed_channels: [], allowed_users: []};
}

function pickBot(bots: DraftBot[], current: string) {
    return current && bots.some((bot) => bot.local_id === current) ? current : (bots[0]?.local_id || '');
}

function serialize(value: unknown) { try { return value == null || value === '' ? '' : typeof value === 'string' ? value : JSON.stringify(value); } catch { return ''; } }
function text(value: unknown) { return typeof value === 'string' ? value : value == null ? '' : String(value); }
function num(value: unknown, fallback: number) { const parsed = Number(value); return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback; }
function auth(value: string) { return value === 'x-api-key' ? 'x-api-key' : 'bearer'; }
function botAuth(value: string) { return value === 'x-api-key' ? 'x-api-key' : value === 'bearer' ? 'bearer' : ''; }
function mode(value: string) { return value === 'enhanced' || value === 'auto' ? value : 'standard'; }
function ocr(value: string) { return value === 'force' ? 'force' : 'auto'; }
function user(value: string) { return value.toLowerCase().replace(/[^a-z0-9-_]/g, ''); }
function id(prefix: string) { return typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function' ? `${prefix}-${crypto.randomUUID()}` : `${prefix}-${Date.now()}-${Math.random().toString(16).slice(2)}`; }
function join(values: string[]) { return values.join(', '); }
function split(value: string, lower = false) { return value.split(/[\r\n,]+/).map((item) => lower ? item.trim().toLowerCase() : item.trim()).filter(Boolean).filter((item, index, all) => all.indexOf(item) === index); }
function formats(values: unknown) { const allowed = new Set(['html', 'markdown', 'text']); return (Array.isArray(values) ? values : []).map((item) => text(item).trim().toLowerCase()).filter((item) => allowed.has(item)).filter((item, index, all) => all.indexOf(item) === index); }
