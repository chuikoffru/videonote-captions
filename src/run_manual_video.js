const { generateVideo } = require('./services/videoService');
const path = require('path');
const fs = require('fs');

async function runManualVideo() {
    const tempDir = path.join(__dirname, '../temp');
    const audioPath = path.join(tempDir, 'test_audio.ogg');
    const avatarPath = path.join(tempDir, 'test_avatar.jpg');
    const srtPath = path.join(tempDir, 'test_transcribe.srt');

    console.log('Starting manual video generation with:');
    console.log('Audio:', audioPath);
    console.log('Avatar:', avatarPath);
    console.log('SRT:', srtPath);

    if (!fs.existsSync(audioPath)) {
        console.error('Audio file not found');
        process.exit(1);
    }
    if (!fs.existsSync(avatarPath)) {
        console.error('Avatar file not found');
        process.exit(1);
    }
    if (!fs.existsSync(srtPath)) {
        console.error('SRT file not found');
        process.exit(1);
    }

    try {
        const videoPath = await generateVideo(audioPath, avatarPath, srtPath);
        console.log('SUCCESS: Video generated at', videoPath);
    } catch (e) {
        console.error('FAILURE: Video generation failed:', e);
        process.exit(1);
    }
}

runManualVideo();
