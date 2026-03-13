import React, {useEffect, useMemo, useRef, useState} from 'react';

import type {WebSocketMessage} from '@mattermost/client';

import PostText from './post_text';

import {isUpstageAwaitingFirstChunk} from '../streaming';

type PostUpdateData = {
    post_id?: string;
    next?: string;
    control?: string;
};

type Props = {
    post: any;
    websocketRegister: (postID: string, listenerID: string, listener: (msg: WebSocketMessage<PostUpdateData>) => void) => void;
    websocketUnregister: (postID: string, listenerID: string) => void;
};

const containerStyle: React.CSSProperties = {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
};

const statusStyle: React.CSSProperties = {
    color: 'rgba(var(--center-channel-color-rgb), 0.72)',
    fontSize: '12px',
    fontWeight: 600,
    letterSpacing: '0.01em',
};

const precontentStyle: React.CSSProperties = {
    alignItems: 'center',
    color: 'rgba(var(--center-channel-color-rgb), 0.72)',
    display: 'inline-flex',
    fontSize: '13px',
    gap: '8px',
};

const spinnerStyle: React.CSSProperties = {
    animation: 'upstage-stream-cursor-blink 700ms linear infinite',
    background: 'rgba(var(--center-channel-color-rgb), 0.16)',
    borderRadius: '999px',
    display: 'inline-block',
    height: '10px',
    width: '10px',
};

const toolbarStyle: React.CSSProperties = {
    alignItems: 'center',
    display: 'flex',
    gap: '8px',
};

const buttonStyle: React.CSSProperties = {
    background: 'rgba(var(--button-bg-rgb), 0.12)',
    border: '1px solid rgba(var(--button-bg-rgb), 0.28)',
    borderRadius: '999px',
    color: 'rgb(var(--button-bg-rgb))',
    cursor: 'pointer',
    fontSize: '12px',
    fontWeight: 600,
    padding: '6px 12px',
};

const modalBackdropStyle: React.CSSProperties = {
    alignItems: 'center',
    background: 'rgba(0, 0, 0, 0.44)',
    bottom: 0,
    display: 'flex',
    justifyContent: 'center',
    left: 0,
    padding: '24px',
    position: 'fixed',
    right: 0,
    top: 0,
    zIndex: 2147483000,
};

const modalCardStyle: React.CSSProperties = {
    background: 'var(--center-channel-bg)',
    border: '1px solid rgba(var(--center-channel-color-rgb), 0.12)',
    borderRadius: '12px',
    boxShadow: '0 16px 48px rgba(0, 0, 0, 0.24)',
    color: 'rgb(var(--center-channel-color-rgb))',
    display: 'flex',
    flexDirection: 'column',
    maxHeight: 'calc(100vh - 48px)',
    overflow: 'hidden',
    width: 'min(960px, calc(100vw - 32px))',
};

const modalHeaderStyle: React.CSSProperties = {
    alignItems: 'center',
    borderBottom: '1px solid rgba(var(--center-channel-color-rgb), 0.12)',
    display: 'flex',
    justifyContent: 'space-between',
    gap: '12px',
    padding: '16px 20px',
};

const modalBodyStyle: React.CSSProperties = {
    display: 'grid',
    gap: '16px',
    gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))',
    overflowY: 'auto',
    padding: '20px',
};

const debugPanelStyle: React.CSSProperties = {
    background: 'rgba(var(--center-channel-color-rgb), 0.04)',
    border: '1px solid rgba(var(--center-channel-color-rgb), 0.08)',
    borderRadius: '10px',
    display: 'flex',
    flexDirection: 'column',
    gap: '12px',
    minHeight: 0,
    padding: '16px',
};

const debugPreStyle: React.CSSProperties = {
    background: 'rgba(var(--center-channel-color-rgb), 0.04)',
    borderRadius: '8px',
    fontSize: '12px',
    margin: 0,
    maxHeight: '52vh',
    minHeight: '160px',
    overflow: 'auto',
    padding: '12px',
    whiteSpace: 'pre-wrap',
    wordBreak: 'break-word',
};

export default function UpstageBotPost(props: Props) {
    const [message, setMessage] = useState(getRenderableMessage(props.post));
    const [generating, setGenerating] = useState(isStreamingPost(props.post));
    const [precontent, setPrecontent] = useState(isUpstageAwaitingFirstChunk(props.post));
    const [showDebugModal, setShowDebugModal] = useState(false);
    const listenerID = useRef(`upstage-${Math.random().toString(36).slice(2)}`);
    const inputDebug = normalizeDebugPayload(props.post?.props?.upstage_request_input || props.post?.props?.upstage_error_input);
    const outputDebug = normalizeDebugPayload(props.post?.props?.upstage_error_output);
    const canShowDebug = inputDebug !== '' || outputDebug !== '';
    const debugButtonLabel = outputDebug !== '' ? '요청/응답 파라미터 보기' : '요청 파라미터 보기';
    const debugModalTitle = outputDebug !== '' ? 'Upstage 요청/응답 파라미터' : 'Upstage 요청 파라미터';

    useEffect(() => {
        setMessage(getRenderableMessage(props.post));
        setGenerating(isStreamingPost(props.post));
        setPrecontent(isUpstageAwaitingFirstChunk(props.post));
        setShowDebugModal(false);
    }, [
        props.post.id,
        props.post.message,
        props.post.props?.upstage_streaming,
        props.post.props?.upstage_stream_status,
        props.post.props?.upstage_stream_placeholder,
        props.post.props?.upstage_request_input,
        props.post.props?.upstage_error_input,
        props.post.props?.upstage_error_output,
    ]);

    useEffect(() => {
        if (!showDebugModal) {
            return undefined;
        }

        const onKeyDown = (event: KeyboardEvent) => {
            if (event.key === 'Escape') {
                setShowDebugModal(false);
            }
        };

        document.addEventListener('keydown', onKeyDown);
        return () => document.removeEventListener('keydown', onKeyDown);
    }, [showDebugModal]);

    const listener = useMemo(() => {
        return (msg: WebSocketMessage<PostUpdateData>) => {
            const data = msg?.data || {};
            if (data.post_id !== props.post.id) {
                return;
            }

            if (data.control === 'start') {
                setGenerating(true);
                setPrecontent(true);
                setMessage('');
                return;
            }

            if (typeof data.next === 'string' && data.next !== '') {
                setGenerating(true);
                setPrecontent(false);
                setMessage(data.next);
                return;
            }

            if (data.control === 'end' || data.control === 'cancel') {
                setGenerating(false);
                setPrecontent(false);
            }
        };
    }, [props.post.id]);

    useEffect(() => {
        props.websocketRegister(props.post.id, listenerID.current, listener);
        return () => {
            props.websocketUnregister(props.post.id, listenerID.current);
        };
    }, [listener, props.post.id, props.websocketRegister, props.websocketUnregister]);

    return (
        <div
            data-testid='upstage-bot-post'
            style={containerStyle}
        >
            {canShowDebug && (
                <div style={toolbarStyle}>
                    <button
                        style={buttonStyle}
                        type='button'
                        onClick={() => setShowDebugModal(true)}
                    >
                        {debugButtonLabel}
                    </button>
                </div>
            )}
            {precontent && (
                <span style={precontentStyle}>
                    <span style={spinnerStyle}/>
                    {'응답 생성 시작 중...'}
                </span>
            )}
            <PostText
                channelID={props.post.channel_id}
                message={message}
                postID={props.post.id}
                showCursor={generating && !precontent}
            />
            {generating && !precontent && (
                <span style={statusStyle}>
                    {'응답 생성 중...'}
                </span>
            )}
            {showDebugModal && canShowDebug && (
                <div
                    aria-modal='true'
                    role='dialog'
                    style={modalBackdropStyle}
                    onClick={() => setShowDebugModal(false)}
                >
                    <div
                        style={modalCardStyle}
                        onClick={(event) => event.stopPropagation()}
                    >
                        <div style={modalHeaderStyle}>
                            <div style={{display: 'flex', flexDirection: 'column', gap: '4px'}}>
                                <strong>{debugModalTitle}</strong>
                                <span style={statusStyle}>
                                    {`Correlation ID: ${props.post?.props?.upstage_correlation_id || '-'}`}
                                </span>
                            </div>
                            <button
                                style={buttonStyle}
                                type='button'
                                onClick={() => setShowDebugModal(false)}
                            >
                                {'닫기'}
                            </button>
                        </div>
                        <div style={modalBodyStyle}>
                            <section style={debugPanelStyle}>
                                <strong>{'Request Parameters'}</strong>
                                <pre style={debugPreStyle}>{inputDebug || '{}'}</pre>
                            </section>
                            {outputDebug !== '' && (
                                <section style={debugPanelStyle}>
                                    <strong>{'Response Parameters'}</strong>
                                    <pre style={debugPreStyle}>{outputDebug}</pre>
                                </section>
                            )}
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}

function isStreamingPost(post: any) {
    return post?.props?.upstage_streaming === 'true' || post?.props?.upstage_stream_status === 'streaming';
}

function getRenderableMessage(post: any) {
    if (isUpstageAwaitingFirstChunk(post)) {
        return '';
    }

    return post?.message || '';
}

function normalizeDebugPayload(value: unknown) {
    if (typeof value !== 'string') {
        return '';
    }

    return value.trim();
}
