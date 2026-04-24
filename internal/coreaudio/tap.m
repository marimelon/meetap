// coreaudio_tap.m - CoreAudio Tap API wrapper for cgo
#import <CoreAudio/CoreAudio.h>
#import <CoreAudio/AudioHardwareTapping.h>
#import <CoreAudio/CATapDescription.h>
#import <Foundation/Foundation.h>
#include "tap.h"

// Get the default output device UID
static NSString* getDefaultOutputDeviceUID() {
    AudioObjectID defaultDevice;
    UInt32 size = sizeof(defaultDevice);
    AudioObjectPropertyAddress addr = {
        kAudioHardwarePropertyDefaultOutputDevice,
        kAudioObjectPropertyScopeGlobal,
        kAudioObjectPropertyElementMain
    };
    OSStatus err = AudioObjectGetPropertyData(kAudioObjectSystemObject, &addr, 0, NULL, &size, &defaultDevice);
    if (err != noErr) return nil;

    CFStringRef uid = NULL;
    size = sizeof(uid);
    AudioObjectPropertyAddress uidAddr = {
        kAudioDevicePropertyDeviceUID,
        kAudioObjectPropertyScopeGlobal,
        kAudioObjectPropertyElementMain
    };
    err = AudioObjectGetPropertyData(defaultDevice, &uidAddr, 0, NULL, &size, &uid);
    if (err != noErr) return nil;

    return (__bridge NSString*)uid;
}

TapResult createGlobalTap() {
    TapResult result = {0};

    @autoreleasepool {
        // Create a global tap excluding nothing (capture all system audio)
        CATapDescription *tapDesc = [[CATapDescription alloc] initStereoGlobalTapButExcludeProcesses:@[]];
        tapDesc.muteBehavior = CATapUnmuted;

        AudioObjectID tapID = 0;
        OSStatus err = AudioHardwareCreateProcessTap(tapDesc, &tapID);
        if (err != noErr) {
            result.error = (int)err;
            return result;
        }

        // Get default output device UID
        NSString *outputUID = getDefaultOutputDeviceUID();
        if (!outputUID) {
            AudioHardwareDestroyProcessTap(tapID);
            result.error = -1;
            return result;
        }

        // Create aggregate device with the tap
        NSString *aggUID = [[NSUUID UUID] UUIDString];
        NSDictionary *tapEntry = @{
            @(kAudioSubTapUIDKey): tapDesc.UUID.UUIDString,
            @(kAudioSubTapDriftCompensationKey): @YES
        };
        NSDictionary *aggDesc = @{
            @(kAudioAggregateDeviceNameKey): @"CoreAudio Tap",
            @(kAudioAggregateDeviceUIDKey): aggUID,
            @(kAudioAggregateDeviceMainSubDeviceKey): outputUID,
            @(kAudioAggregateDeviceTapListKey): @[tapEntry],
            @(kAudioAggregateDeviceIsPrivateKey): @YES
        };

        AudioObjectID aggDevice = 0;
        err = AudioHardwareCreateAggregateDevice((__bridge CFDictionaryRef)aggDesc, &aggDevice);
        if (err != noErr) {
            AudioHardwareDestroyProcessTap(tapID);
            result.error = (int)err;
            return result;
        }

        result.tapID = tapID;
        result.aggregateDeviceID = aggDevice;
        result.error = 0;
    }
    return result;
}

void destroyTap(uint32_t tapID, uint32_t aggregateDeviceID) {
    if (aggregateDeviceID != 0) {
        AudioHardwareDestroyAggregateDevice(aggregateDeviceID);
    }
    if (tapID != 0) {
        AudioHardwareDestroyProcessTap(tapID);
    }
}

void getDeviceName(uint32_t deviceID, char *buf, int bufSize) {
    @autoreleasepool {
        CFStringRef name = NULL;
        UInt32 size = sizeof(name);
        AudioObjectPropertyAddress addr = {
            kAudioObjectPropertyName,
            kAudioObjectPropertyScopeGlobal,
            kAudioObjectPropertyElementMain
        };
        OSStatus err = AudioObjectGetPropertyData(deviceID, &addr, 0, NULL, &size, &name);
        if (err == noErr && name != NULL) {
            CFStringGetCString(name, buf, bufSize, kCFStringEncodingUTF8);
            CFRelease(name);
        } else {
            snprintf(buf, bufSize, "(unknown)");
        }
    }
}
