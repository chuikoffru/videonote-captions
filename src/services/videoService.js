const ffmpeg = require('fluent-ffmpeg');
const ffmpegPath = require('@ffmpeg-installer/ffmpeg').path;
const path = require('path');
const fs = require('fs');
const { promisify } = require('util');
const writeFile = promisify(fs.writeFile);

ffmpeg.setFfmpegPath(ffmpegPath);

/**
 * Создает SRT файл субтитров из транскрипции с временными метками
 * @param {Object} transcription - Объект с текстом и временными метками
 * @param {string} outputPath - Путь для сохранения SRT файла
 * @returns {Promise<string>} Путь к созданному SRT файлу
 */
async function createSubtitleFile(transcription, outputPath) {
  const words = transcription.words || [];
  if (words.length === 0) {
    // Если нет слов, создаем один сегмент с полным текстом
    const srtContent = `1\n00:00:00,000 --> 00:00:05,000\n${transcription.text || 'Без текста'}\n\n`;
    await writeFile(outputPath, srtContent, 'utf-8');
    return outputPath;
  }

  // Группируем слова в фразы (по 5-7 слов или по временным интервалам)
  const phrases = [];
  let currentPhrase = [];
  let phraseStart = words[0].start;
  const maxWordsPerPhrase = 6;
  const maxPhraseDuration = 3.0; // секунды

  for (let i = 0; i < words.length; i++) {
    const word = words[i];
    currentPhrase.push(word.word);

    const shouldBreak =
      currentPhrase.length >= maxWordsPerPhrase ||
      (word.end - phraseStart) >= maxPhraseDuration ||
      i === words.length - 1;

    if (shouldBreak) {
      phrases.push({
        text: currentPhrase.join(' ').trim(),
        start: phraseStart,
        end: word.end
      });
      currentPhrase = [];
      if (i < words.length - 1) {
        phraseStart = words[i + 1].start;
      }
    }
  }

  // Генерируем SRT контент
  let srtContent = '';
  phrases.forEach((phrase, index) => {
    const startTime = formatSRTTime(phrase.start);
    const endTime = formatSRTTime(phrase.end);
    srtContent += `${index + 1}\n${startTime} --> ${endTime}\n${phrase.text}\n\n`;
  });

  await writeFile(outputPath, srtContent, 'utf-8');
  return outputPath;
}

/**
 * Форматирует время в формат SRT (00:00:00,000)
 * @param {number} seconds - Время в секундах
 * @returns {string} Отформатированное время
 */
function formatSRTTime(seconds) {
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = Math.floor(seconds % 60);
  const ms = Math.floor((seconds % 1) * 1000);

  return `${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}:${String(secs).padStart(2, '0')},${String(ms).padStart(3, '0')}`;
}

/**
 * Генерирует видео с визуализацией аудио, аватаркой и субтитрами
 * @param {string} audioPath - Путь к аудиофайлу
 * @param {string} avatarPath - Путь к аватарке
 * @param {Object|string} transcriptionOrSrtPath - Объект с текстом и временными метками ИЛИ путь к готовому SRT файлу
 * @returns {Promise<string>} Путь к сгенерированному видео
 */
async function generateVideo(audioPath, avatarPath, transcriptionOrSrtPath) {
  const tempDir = path.join(__dirname, '../../temp');
  const outputPath = path.join(tempDir, `video_${Date.now()}.mp4`);

  let subtitlePath;
  if (typeof transcriptionOrSrtPath === 'string') {
    // Если передан путь к файлу, используем его
    subtitlePath = transcriptionOrSrtPath;
  } else {
    // Если передан объект транскрипции, генерируем файл
    subtitlePath = path.join(tempDir, `subtitles_${Date.now()}.srt`);
    await createSubtitleFile(transcriptionOrSrtPath, subtitlePath);
  }

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

  // Экранируем путь к субтитрам для FFmpeg
  const escapedSubtitlePath = subtitlePath.replace(/:/g, '\\:').replace(/\\/g, '/');

  return new Promise((resolve, reject) => {
    const command = ffmpeg()
      .input(audioPath)
      .input(avatarPath)
      .complexFilter([
        // Создаем темный градиентный фон
        `color=c=0x1a1a2e:s=${width}x${height}:d=10[bg]`,

        // Создаем градиентный overlay для фона
        `[bg]drawbox=x=0:y=0:w=${width}:h=${height}:color=0x16213e:t=fill[bg_grad]`,

        // Генерируем неоновую визуализацию (showwaves)
        `[0:a]showwaves=s=${width - 100}x${waveformHeight}:mode=line:colors=0x00ffff:rate=${fps}[wave_raw]`,
        `[wave_raw]format=yuva420p[wave]`,

        // Масштабируем и делаем аватарку круглой
        `[1:v]scale=${avatarSize}:${avatarSize},format=yuva420p,` +
        `geq=lum='p(X,Y)':a='if(gt(sqrt(pow(X-${avatarSize / 2},2)+pow(Y-${avatarSize / 2},2)),${avatarSize / 2}),0,255)'[avatar]`,

        // Добавляем светящуюся рамку вокруг аватарки
        `[avatar]drawbox=x=0:y=0:w=${avatarSize}:h=${avatarSize}:color=0x00ffff:t=6[avatar_glow]`,

        // Накладываем аватарку на фон
        `[bg_grad][avatar_glow]overlay=${avatarX}:${avatarY}[with_avatar]`,

        // Накладываем визуализацию
        `[with_avatar][wave]overlay=50:${waveformY}[with_wave]`,

        // Добавляем динамические субтитры из SRT файла (ниже, без обводки)
        `[with_wave]subtitles=${escapedSubtitlePath}:force_style='FontSize=32,PrimaryColour=&HFFFFFF&,BorderStyle=0,Outline=0,Shadow=0,Alignment=2,MarginV=130'[with_subs]`,

        // Добавляем название бота под субтитрами
        `[with_subs]drawtext=fontfile=${process.platform === 'darwin' ? '/System/Library/Fonts/Helvetica.ttc' : '/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf'}:text='@AudioVisualizerBot':fontcolor=0x00ffff:fontsize=24:x=(w-text_w)/2:y=h-80[final]`
      ])
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
