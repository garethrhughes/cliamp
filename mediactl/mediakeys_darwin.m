//go:build darwin

#import <MediaPlayer/MediaPlayer.h>
#import <Foundation/Foundation.h>
#include "mediakeys_darwin.h"

// Go callback declarations (implemented in mediakeys_darwin.go via //export).
extern void goMediaKeyPlay(void);
extern void goMediaKeyPause(void);
extern void goMediaKeyToggle(void);
extern void goMediaKeyNext(void);
extern void goMediaKeyPrev(void);
extern void goMediaKeyStop(void);
extern void goMediaKeySeekTo(double positionSec);

static BOOL initialized = NO;

void MediaKeysInit(void) {
    if (initialized) return;
    initialized = YES;

    MPRemoteCommandCenter *center = [MPRemoteCommandCenter sharedCommandCenter];

    [center.playCommand addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
        goMediaKeyPlay();
        return MPRemoteCommandHandlerStatusSuccess;
    }];

    [center.pauseCommand addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
        goMediaKeyPause();
        return MPRemoteCommandHandlerStatusSuccess;
    }];

    [center.togglePlayPauseCommand addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
        goMediaKeyToggle();
        return MPRemoteCommandHandlerStatusSuccess;
    }];

    [center.nextTrackCommand addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
        goMediaKeyNext();
        return MPRemoteCommandHandlerStatusSuccess;
    }];

    [center.previousTrackCommand addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
        goMediaKeyPrev();
        return MPRemoteCommandHandlerStatusSuccess;
    }];

    [center.stopCommand addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
        goMediaKeyStop();
        return MPRemoteCommandHandlerStatusSuccess;
    }];

    [center.changePlaybackPositionCommand addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
        MPChangePlaybackPositionCommandEvent *posEvent = (MPChangePlaybackPositionCommandEvent *)event;
        goMediaKeySeekTo(posEvent.positionTime);
        return MPRemoteCommandHandlerStatusSuccess;
    }];

    center.playCommand.enabled = YES;
    center.pauseCommand.enabled = YES;
    center.togglePlayPauseCommand.enabled = YES;
    center.nextTrackCommand.enabled = YES;
    center.previousTrackCommand.enabled = YES;
    center.stopCommand.enabled = YES;
    center.changePlaybackPositionCommand.enabled = YES;
}

void MediaKeysUpdateNowPlaying(const char *title, const char *artist,
                               const char *album, double durationSec,
                               double positionSec, bool playing) {
    NSMutableDictionary *info = [NSMutableDictionary dictionary];

    if (title)  info[MPMediaItemPropertyTitle]     = [NSString stringWithUTF8String:title];
    if (artist) info[MPMediaItemPropertyArtist]    = [NSString stringWithUTF8String:artist];
    if (album)  info[MPMediaItemPropertyAlbumTitle] = [NSString stringWithUTF8String:album];

    if (durationSec > 0) {
        info[MPMediaItemPropertyPlaybackDuration] = @(durationSec);
    }
    info[MPNowPlayingInfoPropertyElapsedPlaybackTime] = @(positionSec);
    info[MPNowPlayingInfoPropertyPlaybackRate]        = playing ? @(1.0) : @(0.0);

    [MPNowPlayingInfoCenter defaultCenter].nowPlayingInfo = info;
}

void MediaKeysUpdatePosition(double positionSec) {
    NSMutableDictionary *info = [[MPNowPlayingInfoCenter defaultCenter].nowPlayingInfo mutableCopy];
    if (!info) return;
    info[MPNowPlayingInfoPropertyElapsedPlaybackTime] = @(positionSec);
    [MPNowPlayingInfoCenter defaultCenter].nowPlayingInfo = info;
}

void MediaKeysClose(void) {
    if (!initialized) return;

    MPRemoteCommandCenter *center = [MPRemoteCommandCenter sharedCommandCenter];
    [center.playCommand removeTarget:nil];
    [center.pauseCommand removeTarget:nil];
    [center.togglePlayPauseCommand removeTarget:nil];
    [center.nextTrackCommand removeTarget:nil];
    [center.previousTrackCommand removeTarget:nil];
    [center.stopCommand removeTarget:nil];
    [center.changePlaybackPositionCommand removeTarget:nil];

    [MPNowPlayingInfoCenter defaultCenter].nowPlayingInfo = nil;
    initialized = NO;
}
