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
 * @param {string} username - Имя пользователя или @username для отображения
 * @param {Function} onProgress - Callback функция для отслеживания прогресса (опционально)
 * @returns {Promise<string>} Путь к сгенерированному видео
 */
async function generateVideo(audioPath, avatarPath, transcriptionOrSrtPath, username = '@username', onProgress) {
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

  // Путь к файлу шрифта (Noto Sans поддерживает больше символов, чем Liberation)
  const fontPath = process.platform === 'darwin'
    ? '/System/Library/Fonts/Helvetica.ttc'
    : '/usr/share/fonts/truetype/noto/NotoSans-Regular.ttf';

  // Имя шрифта для субтитров (должно быть зарегистрировано в системе)
  const fontName = process.platform === 'darwin' ? 'Helvetica' : 'Noto Sans';

  // Экранируем путь к субтитрам для FFmpeg
  const escapedSubtitlePath = subtitlePath.replace(/:/g, '\\:').replace(/\\/g, '/');

  // Форматируем юзернейм (добавляем @ если нет)
  const displayUsername = username.startsWith('@') ? username : `@${username}`;

  return new Promise((resolve, reject) => {
    const command = ffmpeg()
      .input(audioPath)
      .input(avatarPath)
      .complexFilter([
        // 1. Фон (bg)
        `color=c=0x1a1a2e:s=${width}x${height}:d=10[bg]`,
        `[bg]drawbox=x=0:y=0:w=${width}:h=${height}:color=0x16213e:t=fill[bg_grad]`,

        // 2. Аватар (avatar) -> [with_avatar]
        // Масштабируем и делаем круглым c рамкой "внутри" круга
        // Используем RGBA для удобной работы с цветами
        `[1:v]scale=${avatarSize}:${avatarSize},format=rgba,` +
        `geq=r='if(gt(hypot(X-${avatarSize / 2},Y-${avatarSize / 2}),${avatarSize / 2 - 6}),0,p(X,Y))':` +
        `g='if(gt(hypot(X-${avatarSize / 2},Y-${avatarSize / 2}),${avatarSize / 2 - 6}),255,p(X,Y))':` +
        `b='if(gt(hypot(X-${avatarSize / 2},Y-${avatarSize / 2}),${avatarSize / 2 - 6}),255,p(X,Y))':` +
        `a='if(gt(hypot(X-${avatarSize / 2},Y-${avatarSize / 2}),${avatarSize / 2}),0,255)'[avatar_round]`,

        // Накладываем аватар на фон (повыше, Y=80)
        `[bg_grad][avatar_round]overlay=${avatarX}:80[with_avatar]`,

        // Добавляем юзернейм под аватаром (Y=290, т.е. 80 + 200 + 10)
        `[with_avatar]drawtext=fontfile=${fontPath}:text='${displayUsername}':fontcolor=white:fontsize=24:x=(w-text_w)/2:y=290[with_username]`,

        // 3. Визуализация (bars) -> [with_wave]
        // Усиливаем звук для визуализации, чтобы столбики прыгали активнее
        `[0:a]volume=8.0[loud_a]`,

        // 1. Генерируем 64 столбика (низкое разрешение)
        `[loud_a]showfreqs=s=64x${waveformHeight / 2}:mode=bar:ascale=log:fscale=log:colors=0x00ffff[freqs_base]`,

        // 2. Убеждаемся в формате yuv420p
        `[freqs_base]format=yuv420p[freqs_base_yuv]`,

        // 3. Масштабируем x2 по ширине (до 128px) без сглаживания
        `[freqs_base_yuv]scale=128:${waveformHeight / 2}:flags=neighbor[freqs_x2]`,

        // 4. Делаем "дырки": закрашиваем черным каждый второй столбец
        // Принудительно ставим Y=0, U=128, V=128 (черный цвет), чтобы не было зеленых артефактов
        `[freqs_x2]geq=lum='if(mod(X,2),0,p(X,Y))':cb='if(mod(X,2),128,p(X,Y))':cr='if(mod(X,2),128,p(X,Y))'[freqs_holes]`,

        // 5. Масштабируем до итоговой ширины
        `[freqs_holes]scale=${width - 100}:${waveformHeight / 2}:flags=neighbor[freqs_spaced]`,

        // 6. Сглаживаем движение
        `[freqs_spaced]tmix=frames=3[freqs_smooth]`,

        // 7. Разделяем и делаем зеркальное отражение
        `[freqs_smooth]split[freqs_top][freqs_mirror_src]`,
        `[freqs_mirror_src]vflip[freqs_bottom]`,
        `[freqs_top][freqs_bottom]vstack[freqs_full]`,

        // 8. Финальная очистка черного (для надежности)
        `[freqs_full]colorkey=0x000000:0.01:0.1[freqs_transparent]`,

        // Накладываем визуализацию под аватаром (Y=350)
        `[with_username][freqs_transparent]overlay=50:350[with_wave]`,

        // 4. Субтитры (subs) -> [with_subs]
        // Убираем обводку (Outline=0, Shadow=0)
        `[with_wave]subtitles=${escapedSubtitlePath}:force_style='FontName=${fontName},FontSize=30,PrimaryColour=&HFFFFFF&,BorderStyle=0,Outline=0,Shadow=0,Alignment=2,MarginV=20'[with_subs]`,

        // 5. Имя бота -> [final]
        `[with_subs]drawtext=fontfile=${fontPath}:text='@AudioVisualizerBot':fontcolor=0x00ffff:fontsize=20:x=(w-text_w)/2:y=h-30[final]`
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
          const percent = Math.floor(progress.percent);
          console.log(`Обработка: ${percent}%`);
          if (onProgress) {
            onProgress(percent);
          }
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
