import type {Post} from '@mattermost/types/posts';
import type {GlobalState} from '@mattermost/types/store';

jest.mock('mattermost-redux/actions/posts', () => ({
    receivedPost: jest.fn((post) => ({
        type: 'RECEIVED_POST',
        data: post,
        features: {
            crtEnabled: undefined,
        },
    })),
}), {virtual: true});

import {buildPluginWebSocketEventName, buildStreamingPostUpdate, isUpstageAwaitingFirstChunk} from './streaming';

function makeState(post: Post) {
    return {
        entities: {
            posts: {
                posts: {
                    [post.id]: post,
                },
            },
        },
    } as GlobalState;
}

function makePost(overrides: Partial<Post> = {}) {
    return {
        id: 'post-id',
        channel_id: 'channel-id',
        create_at: 1,
        delete_at: 0,
        edit_at: 0,
        file_ids: [],
        hashtags: '',
        is_pinned: false,
        message: 'initial',
        metadata: {},
        original_id: '',
        pending_post_id: '',
        props: {
            upstage_stream: 'true',
            upstage_streaming: 'true',
        },
        root_id: '',
        type: '',
        update_at: 1,
        user_id: 'bot-user-id',
        ...overrides,
    } as Post;
}

test('buildPluginWebSocketEventName prefixes plugin events correctly', () => {
    expect(buildPluginWebSocketEventName('com.mattermost.upstage-document-parser', 'postupdate')).toBe('custom_com.mattermost.upstage-document-parser_postupdate');
});

test('buildStreamingPostUpdate updates only streaming Upstage posts', () => {
    const post = makePost();

    const updatedPost = buildStreamingPostUpdate(makeState(post), {
        post_id: post.id,
        next: 'streamed reply',
    });

    expect(updatedPost).toBeTruthy();
    expect(updatedPost?.message).toBe('streamed reply');
    expect(updatedPost?.props?.upstage_stream_status).toBe('streaming');
});

test('buildStreamingPostUpdate ignores non-streaming posts', () => {
    const post = makePost({
        props: {},
    });

    const updatedPost = buildStreamingPostUpdate(makeState(post), {
        post_id: post.id,
        next: 'streamed reply',
    });

    expect(updatedPost).toBeNull();
});

test('isUpstageAwaitingFirstChunk detects placeholder streaming posts', () => {
    const post = makePost({
        props: {
            upstage_stream: 'true',
            upstage_streaming: 'true',
            upstage_stream_placeholder: 'true',
        },
    });

    expect(isUpstageAwaitingFirstChunk(post)).toBe(true);
});
