import type {Store} from 'redux';

import type {WebSocketMessage} from '@mattermost/client';
import type {Post} from '@mattermost/types/posts';
import type {GlobalState} from '@mattermost/types/store';

import {receivedPost} from 'mattermost-redux/actions/posts';

import type {PluginRegistry} from './types/mattermost-webapp';

type StreamingPostUpdateEventData = {
    post_id?: string;
    next?: string;
    control?: string;
};

export type {StreamingPostUpdateEventData};

export function buildPluginWebSocketEventName(pluginID: string, eventName: string) {
    return `custom_${pluginID}_${eventName}`;
}

export function isUpstageStreamingPost(post?: Post | null): post is Post {
    if (!post) {
        return false;
    }

    const props = post.props || {};
    return props.upstage_stream === 'true' || props.upstage_streaming === 'true';
}

export function isUpstageAwaitingFirstChunk(post?: Post | null) {
    if (!post) {
        return false;
    }

    const props = post.props || {};
    return isUpstageStreamingPost(post) && props.upstage_stream_placeholder === 'true';
}

export function buildStreamingPostUpdate(state: GlobalState, data?: StreamingPostUpdateEventData): Post | null {
    const postID = normalizeIdentifier(data?.post_id);
    const nextMessage = typeof data?.next === 'string' ? data.next : '';
    if (!postID || nextMessage.trim() === '') {
        return null;
    }

    const existingPost = state.entities.posts.posts[postID];
    if (!isUpstageStreamingPost(existingPost) || existingPost.message === nextMessage) {
        return null;
    }

    return {
        ...existingPost,
        message: nextMessage,
        update_at: Date.now(),
        props: {
            ...existingPost.props,
            upstage_stream: 'true',
            upstage_streaming: 'true',
            upstage_stream_status: 'streaming',
            upstage_stream_placeholder: 'false',
        },
    };
}

export function handleStreamingPostUpdateEvent(
    store: Store<GlobalState>,
    msg: WebSocketMessage<StreamingPostUpdateEventData>,
) {
    if (!msg?.data) {
        return;
    }

    const updatedPost = buildStreamingPostUpdate(store.getState(), msg.data);
    if (!updatedPost) {
        return;
    }

    store.dispatch(receivedPost(updatedPost) as any);
}

export function registerStreamingPostHandler(
    registry: Pick<PluginRegistry, 'registerWebSocketEventHandler'>,
    store: Store<GlobalState>,
    pluginID: string,
) {
    registry.registerWebSocketEventHandler(
        buildPluginWebSocketEventName(pluginID, 'postupdate'),
        (msg: WebSocketMessage<StreamingPostUpdateEventData>) => handleStreamingPostUpdateEvent(store, msg),
    );
}

function normalizeIdentifier(value?: string) {
    if (typeof value !== 'string') {
        return '';
    }

    return value.trim();
}
