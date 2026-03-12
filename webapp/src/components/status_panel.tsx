import React, {useEffect, useState} from 'react';

import type {ConnectionStatus, PluginStatus} from '../client';
import {getStatus, testConnection} from '../client';

export default function StatusPanel() {
    const [status, setStatus] = useState<PluginStatus | null>(null);
    const [connection, setConnection] = useState<ConnectionStatus | null>(null);
    const [loading, setLoading] = useState(true);
    const [testing, setTesting] = useState(false);
    const [message, setMessage] = useState('');

    useEffect(() => {
        let cancelled = false;
        async function load() {
            try {
                const next = await getStatus();
                if (!cancelled) {
                    setStatus(next);
                }
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
    }, []);

    async function onTest() {
        setTesting(true);
        setMessage('');
        try {
            setConnection(await testConnection());
        } catch (e) {
            setMessage((e as Error).message);
        } finally {
            setTesting(false);
        }
    }

    return <div style={{display: 'flex', flexDirection: 'column', gap: 12, padding: 16}}>
        <strong>{'Upstage 상태'}</strong>
        {loading && <span>{'플러그인 상태를 불러오는 중입니다...'}</span>}
        {status && <><div>{`기본 URL: ${status.base_url || '설정되지 않음'}`}</div><div>{`봇 수: ${status.bot_count}`}</div><div>{`허용 호스트: ${(status.allow_hosts || []).join(', ') || '기본 URL 호스트 사용'}`}</div>{status.config_error && <div>{`설정 오류: ${status.config_error}`}</div>}{status.bot_sync?.last_error && <div>{`동기화 오류: ${status.bot_sync.last_error}`}</div>}</>}
        <button className='btn btn-primary' type='button' disabled={testing} onClick={onTest}>{testing ? '연결 확인 중...' : '연결 테스트'}</button>
        {connection && <div><div>{connection.ok ? '연결에 성공했습니다.' : '연결에 실패했습니다.'}</div><div>{connection.url}</div><div style={{whiteSpace: 'pre-wrap'}}>{connection.message}</div></div>}
        {message && <span>{message}</span>}
    </div>;
}
