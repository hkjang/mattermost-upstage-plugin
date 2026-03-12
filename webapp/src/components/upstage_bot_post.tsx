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

export default function UpstageBotPost(props: Props) {
    const [message, setMessage] = useState(getRenderableMessage(props.post));
    const [generating, setGenerating] = useState(isStreamingPost(props.post));
    const [precontent, setPrecontent] = useState(isUpstageAwaitingFirstChunk(props.post));
    const listenerID = useRef(`upstage-${Math.random().toString(36).slice(2)}`);

    useEffect(() => {
        setMessage(getRenderableMessage(props.post));
        setGenerating(isStreamingPost(props.post));
        setPrecontent(isUpstageAwaitingFirstChunk(props.post));
    }, [
        props.post.message,
        props.post.props?.upstage_streaming,
        props.post.props?.upstage_stream_status,
        props.post.props?.upstage_stream_placeholder,
    ]);

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
