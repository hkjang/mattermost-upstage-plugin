import React, {useEffect, useMemo, useState} from 'react';
import {useSelector} from 'react-redux';

import type {GlobalState} from '@mattermost/types/store';

import type {BotDefinition, BotRunResult, ExecutionRecord} from '../client';
import {getBots, getHistory, runBot} from '../client';

const card: React.CSSProperties = {background: 'rgba(var(--center-channel-color-rgb),.04)', border: '1px solid rgba(var(--center-channel-color-rgb),.12)', borderRadius: 12, padding: 12, display: 'flex', flexDirection: 'column', gap: 8};
const field: React.CSSProperties = {width: '100%', border: '1px solid rgba(var(--center-channel-color-rgb),.16)', borderRadius: 8, padding: '10px 12px'};

export default function RHSPane() {
    const channelId = useSelector((state: GlobalState) => state.entities.channels.currentChannelId);
    const selectedPostId = useSelector((state: GlobalState) => (state as any).views?.rhs?.selectedPostId as string | undefined);
    const post = useSelector((state: GlobalState) => selectedPostId ? ((state as any).entities?.posts?.posts || {})[selectedPostId] : null) as any;
    const files = useSelector((state: GlobalState) => ((state as any).entities?.files?.files || {})) as Record<string, any>;

    const [bots, setBots] = useState<BotDefinition[]>([]);
    const [history, setHistory] = useState<ExecutionRecord[]>([]);
    const [selectedBotId, setSelectedBotId] = useState('');
    const [prompt, setPrompt] = useState('');
    const [message, setMessage] = useState('');
    const [loading, setLoading] = useState(true);
    const [submitting, setSubmitting] = useState(false);
    const [lastResult, setLastResult] = useState<BotRunResult | null>(null);

    const bot = useMemo(() => bots.find((item) => item.id === selectedBotId) || bots[0] || null, [bots, selectedBotId]);
    const fileIds = Array.isArray(post?.file_ids) ? post.file_ids.filter(Boolean) : [];
    const fileNames = fileIds.map((id: string) => files[id]?.name || id);
    const rootId = post?.root_id || post?.id || selectedPostId;

    useEffect(() => {
        let cancelled = false;
        async function load() {
            setLoading(true);
            setMessage('');
            try {
                const [nextBots, nextHistory] = await Promise.all([getBots(channelId), getHistory(5)]);
                if (cancelled) {
                    return;
                }
                setBots(nextBots);
                setHistory(nextHistory);
                setSelectedBotId((current) => current && nextBots.some((item) => item.id === current) ? current : (nextBots[0]?.id || ''));
            } catch (e) {
                if (!cancelled) {
                    setMessage((e as Error).message);
                }
            } finally {
                if (!cancelled) {
                    setLoading(false);
                }
            }
        }
        void load();
        return () => { cancelled = true; };
    }, [channelId]);

    async function submit() {
        if (!bot || !channelId || fileIds.length === 0) {
            return;
        }
        setSubmitting(true);
        setMessage('');
        try {
            const result = await runBot({bot_id: bot.id, channel_id: channelId, root_id: rootId, prompt, file_ids: fileIds});
            setLastResult(result);
            setPrompt('');
            setHistory(await getHistory(5));
            setMessage(`@${bot.username} 봇이 Mattermost 스레드에 응답을 게시했습니다.`);
        } catch (e) {
            setMessage((e as Error).message);
        } finally {
            setSubmitting(false);
        }
    }

    return <div style={{display: 'flex', flexDirection: 'column', gap: 16, padding: 16}}>
        <section style={card}>
            <strong>{'Upstage Document Parser'}</strong>
            <span style={{fontSize: 12, opacity: .8}}>{'현재 선택한 포스트의 첨부 파일만 Upstage API의 document 파트로 전송합니다. 메시지는 파싱 입력으로 전달되지 않습니다.'}</span>
            {loading && <span>{'봇 목록을 불러오는 중입니다...'}</span>}
            {!loading && bots.length === 0 && <span>{'현재 채널에서 사용할 수 있는 Upstage 봇이 없습니다.'}</span>}
            {!loading && bots.length > 0 && <>
                <select style={field} value={bot?.id || ''} onChange={(e) => setSelectedBotId(e.target.value)}>
                    {bots.map((item) => <option key={item.id} value={item.id}>{`${item.display_name || item.username} (@${item.username})`}</option>)}
                </select>
                <div style={{fontSize: 12, opacity: .8}}>{`Model: ${bot?.model || 'document-parse'} | mode=${bot?.mode || 'standard'} | ocr=${bot?.ocr || 'auto'}`}</div>
                {bot?.description && <span style={{opacity: .8}}>{bot.description}</span>}
                <div style={{fontSize: 12, opacity: .8}}>{selectedPostId ? (fileNames.length > 0 ? `첨부 파일: ${fileNames.join(', ')}` : '첨부 파일이 없습니다. 파일이 있는 포스트를 선택해 주세요.') : '포스트에서 RHS를 열면 해당 첨부 파일을 바로 파싱할 수 있습니다.'}</div>
                <textarea style={{...field, resize: 'vertical'}} rows={5} value={prompt} placeholder={'예: 표를 markdown으로 정리해줘'} onChange={(e) => setPrompt(e.target.value)}/>
                <button className='btn btn-primary' type='button' disabled={submitting || !bot || !channelId || fileIds.length === 0} onClick={submit}>{submitting ? '파싱 요청 중...' : `@${bot?.username || 'bot'}로 실행`}</button>
            </>}
            {message && <span>{message}</span>}
        </section>

        {lastResult && <section style={card}>
            <strong>{'최근 실행 결과'}</strong>
            <div>{`${lastResult.bot_name || lastResult.bot_username} - ${lastResult.status}`}</div>
            <div>{`Model: ${lastResult.model}`}</div>
            {lastResult.output && <div style={{fontSize: 12, opacity: .8, whiteSpace: 'pre-wrap'}}>{cut(lastResult.output, 400)}</div>}
            {lastResult.error_message && <div style={{whiteSpace: 'pre-wrap'}}>{lastResult.error_message}</div>}
            {lastResult.error_code && <div>{`Code: ${lastResult.error_code}`}</div>}
            {lastResult.request_url && <div style={{wordBreak: 'break-all'}}>{`URL: ${lastResult.request_url}`}</div>}
            {lastResult.correlation_id && <div>{`Correlation: ${lastResult.correlation_id}`}</div>}
        </section>}

        <section style={card}>
            <strong>{'최근 기록'}</strong>
            {history.length === 0 && <span>{'아직 실행 기록이 없습니다.'}</span>}
            {history.map((item) => <div key={item.correlation_id} style={{fontSize: 12}}><strong>{item.bot_name || item.bot_username}</strong><div>{`@${item.bot_username} -> ${item.model}`}</div><div>{`${item.status} via ${item.source}`}</div>{item.error_message && <div style={{whiteSpace: 'pre-wrap'}}>{item.error_message}</div>}</div>)}
        </section>
    </div>;
}

function cut(value: string, max: number) {
    const next = (value || '').trim();
    return next.length <= max ? next : `${next.slice(0, max - 1)}…`;
}
