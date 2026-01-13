const ffmpeg = require('fluent-ffmpeg');
const ffmpegPath = require('@ffmpeg-installer/ffmpeg').path;
const path = require('path');
const fs = require('fs');

ffmpeg.setFfmpegPath(ffmpegPath);

/**
 * Генерирует видео с визуализацией аудио, аватаркой и субтитрами
 * @param {string} audioPath - Путь к аудиофайлу
 * @param {string} avatarPath - Путь к аватарке
 * @param {Object} transcription - Объект с текстом и временными метками
 * @returns {Promise<string>} Путь к сгенерированному видео
 */
async function generateVideo(audioPath, avatarPath, transcription) {
  const tempDir = path.join(__dirname, '../../temp');
  const outputPath = path.join(tempDir, `video_${Date.now()}.mp4`);
  
  const width = parseInt(process.env.VIDEO_WIDTH) || 1280;
  const height = parseInt(process.env.VIDEO_HEIGHT) || 720;
  const fps = parseInt(process.env.VIDEO_FPS) || 30;
  const bitrate = process.env.VIDEO_BITRATE || '2000k';
  
  // Размеры и позиции элементов
  const avatarSize = 200;
  const avatarX = (width - avatarSize) / 2;
  const avatarY = 150;
  
  const waveformHeight = 120;
  const waveformY = avatarY + avatarSize + 40;
  
  // Форматируем субтитры для отображения
  const subtitleText = transcription.text || 'Без текста';
  const escapedText = subtitleText
    .replace(/'/g, "'\\\\\\''")
    .replace(/:/g, '\\:')
    .replace(/\[/g, '\\[')
    .replace(/\]/g, '\\]');
  
  return new Promise((resolve, reject) => {
    const command = ffmpeg()
      .input(audioPath)
      .input(avatarPath)
      .complexFilter([
        // Создаем темный градиентный фон
        `color=c=0x1a1a2e:s=${width}x${height}:d=10[bg]`,
        
        // Создаем градиентный overlay для фона
        `[bg]drawbox=x=0:y=0:w=${width}:h=${height}:color=0x16213e@0.8:t=fill[bg_grad]`,
        
        // Генерируем неоновую визуализацию аудио (showwaves)
        `[0:a]showwaves=s=${width-100}x${waveformHeight}:mode=cline:colors=0x00ffff|0xff00ff|0x00ff00:scale=sqrt[wave]`,
        
        // Масштабируем и делаем аватарку круглой
        `[1:v]scale=${avatarSize}:${avatarSize},format=yuva420p,` +
        `geq=lum='p(X,Y)':a='if(gt(sqrt(pow(X-${avatarSize/2},2)+pow(Y-${avatarSize/2},2)),${avatarSize/2}),0,255)'[avatar]`,
        
        // Добавляем светящуюся рамку вокруг аватарки
        `[avatar]drawbox=x=0:y=0:w=${avatarSize}:h=${avatarSize}:color=0x00ffff@0.6:t=6[avatar_glow]`,
        
        // Накладываем аватарку на фон
        `[bg_grad][avatar_glow]overlay=${avatarX}:${avatarY}[with_avatar]`,
        
        // Накладываем визуализацию
        `[with_avatar][wave]overlay=50:${waveformY}[with_wave]`,
        
        // Добавляем субтитры с неоновым свечением
        `[with_wave]drawtext=fontfile=/System/Library/Fonts/Helvetica.ttc:` +
        `text='${escapedText}':fontcolor=white:fontsize=32:` +
        `x=(w-text_w)/2:y=${waveformY + waveformHeight + 60}:` +
        `borderw=2:bordercolor=0x00ffff:` +
        `shadowcolor=0x00ffff@0.8:shadowx=0:shadowy=0[final]`
      ], 'final')
      .outputOptions([
        '-map', '[final]',
        '-map', '0:a',
        '-c:v', 'libx264',
        '-preset', 'medium',
        '-crf', '23',
        '-pix_fmt', 'yuv420p',
        `-b:v ${bitrate}`,
        `-r ${fps}`,
        '-c:a', 'aac',
        '-b:a', '192k',
        '-shortest'
      ])
      .output(outputPath)
      .on('start', (commandLine) => {
        console.log('FFmpeg запущен:', commandLine);
      })
      .on('progress', (progress) => {
        if (progress.percent) {
          console.log(`Обработка: ${Math.floor(progress.percent)}%`);
        }
      })
      .on('end', () => {
        console.log('Видео успешно создано');
        resolve(outputPath);
      })
      .on('error', (err) => {
        console.error('Ошибка FFmpeg:', err);
        reject(err);
      })
      .run();
  });
}

module.exports = { generateVideo };
