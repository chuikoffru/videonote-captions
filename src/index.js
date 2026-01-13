require('dotenv').config();
const { Bot, InputFile } = require('grammy');
const fs = require('fs').promises;
const path = require('path');
const { downloadAudio } = require('./services/audioService');
const { transcribeAudio } = require('./services/transcriptionService');
const { generateVideo } = require('./services/videoService');
const { downloadAvatar } = require('./utils/avatarUtils');

const bot = new Bot(process.env.BOT_TOKEN);

// Обработчик команды /start
bot.command('start', async (ctx) => {
  await ctx.reply(
    '👋 Привет! Отправь мне голосовое сообщение или перешли чужое, ' +
    'и я создам видео с визуализацией аудио, аватаркой и субтитрами!'
  );
});

// Обработчик голосовых сообщений
bot.on('message:voice', async (ctx) => {
  await handleVoiceMessage(ctx, ctx.message.voice);
});

// Обработчик пересланных голосовых сообщений
bot.on('message:forward_origin', async (ctx) => {
  if (ctx.message.voice) {
    await handleVoiceMessage(ctx, ctx.message.voice);
  }
});

async function handleVoiceMessage(ctx, voice) {
  const statusMsg = await ctx.reply('⏳ Обрабатываю голосовое сообщение...');
  
  try {
    // Скачиваем аудио
    await ctx.api.editMessageText(
      ctx.chat.id,
      statusMsg.message_id,
      '⬇️ Загружаю аудио...'
    );
    const audioPath = await downloadAudio(ctx, voice);
    
    // Транскрибируем аудио в текст
    await ctx.api.editMessageText(
      ctx.chat.id,
      statusMsg.message_id,
      '📝 Создаю субтитры...'
    );
    const transcription = await transcribeAudio(audioPath);
    
    // Скачиваем аватарку пользователя
    await ctx.api.editMessageText(
      ctx.chat.id,
      statusMsg.message_id,
      '🖼️ Загружаю аватар...'
    );
    const avatarPath = await downloadAvatar(ctx);
    
    // Генерируем видео
    await ctx.api.editMessageText(
      ctx.chat.id,
      statusMsg.message_id,
      '🎬 Создаю видео... (это может занять некоторое время)'
    );
    const videoPath = await generateVideo(audioPath, avatarPath, transcription);
    
    // Отправляем видео
    await ctx.api.editMessageText(
      ctx.chat.id,
      statusMsg.message_id,
      '📤 Отправляю видео...'
    );
    await ctx.replyWithVideo(new InputFile(videoPath));
    
    // Удаляем сообщение о статусе
    await ctx.api.deleteMessage(ctx.chat.id, statusMsg.message_id);
    
    // Очищаем временные файлы
    await cleanupFiles([audioPath, avatarPath, videoPath]);
    
  } catch (error) {
    console.error('Ошибка при обработке:', error);
    await ctx.api.editMessageText(
      ctx.chat.id,
      statusMsg.message_id,
      '❌ Произошла ошибка при обработке. Попробуйте ещё раз.'
    );
  }
}

async function cleanupFiles(filePaths) {
  for (const filePath of filePaths) {
    try {
      await fs.unlink(filePath);
    } catch (error) {
      console.error(`Не удалось удалить файл ${filePath}:`, error.message);
    }
  }
}

// Обработка ошибок
bot.catch((err) => {
  console.error('Ошибка бота:', err);
});

// Запуск бота
bot.start();
console.log('✅ Бот запущен!');
