require('dotenv').config();
const { Bot, InputFile } = require('grammy');
const fs = require('fs').promises;
const path = require('path');
const { downloadAudio } = require('./services/audioService');
const { transcribeAudio } = require('./services/transcriptionService');
const { generateVideo } = require('./services/videoService');
const { downloadAvatar } = require('./utils/avatarUtils');

const bot = new Bot(process.env.BOT_TOKEN);
const pendingInvoices = new Map();

// Хранилище для ожидающих оплаты file_id (чтобы не превышать лимит payload и для валидации)
// Ключ: unique_payload_id, Значение: file_id

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

// Обработчик pre_checkout_query (необходим для оплаты)
bot.on('pre_checkout_query', async (ctx) => {
  const payload = ctx.preCheckoutQuery.invoice_payload;
  // Проверяем, есть ли такой заказ в памяти
  if (!pendingInvoices.has(payload)) {
    return ctx.answerPreCheckoutQuery(false, {
      error_message: '❌ Заказ устарел или бот был перезагружен. Пожалуйста, отправьте сообщение снова.'
    });
  }
  await ctx.answerPreCheckoutQuery(true);
});

// Обработчик успешного платежа
bot.on('message:successful_payment', async (ctx) => {
  const payload = ctx.message.successful_payment.invoice_payload;

  const fileId = pendingInvoices.get(payload);
  if (!fileId) {
    return ctx.reply('❌ Ошибка: файл не найден (возможно, бот был перезагружен). Перешлите сообщение снова.');
  }

  // Удаляем из памяти после успешной оплаты
  pendingInvoices.delete(payload);

  // Создаем объект voice с file_id
  const voice = {
    file_id: fileId,
    duration: 0
  };

  await ctx.reply('✅ Оплата прошла успешно! Начинаю генерацию видео.');
  await handleVoiceMessage(ctx, voice, true);
});

async function handleVoiceMessage(ctx, voice, isPaid = false) {
  // Проверяем длительность и оплату
  if (!isPaid && voice.duration > 60) {
    const duration = voice.duration;
    // Расчет стоимости: 10 звезд за каждые 60 секунд (округление вверх)
    const price = Math.ceil(duration / 60) * 10;

    // Генерируем уникальный ID для payload
    const payloadId = `${ctx.from.id}_${Date.now()}`;
    pendingInvoices.set(payloadId, voice.file_id);

    await ctx.api.sendInvoice(
      ctx.chat.id,
      'Длинное голосовое сообщение',
      `Первые 60 секунд — бесплатно! Каждые следующие 60 секунд — 10 звезд. Стоимость твоего видео ${price} Telegram Stars`,
      payloadId, // payload
      'XTR', // currency
      [{ label: 'Видео', amount: price }], // prices
      { provider_token: '' } // options
    );
    return;
  }

  const statusMsg = await ctx.reply('⏳ Обрабатываю голосовое сообщение...');

  try {
    // Скачиваем аудио
    await ctx.api.editMessageText(
      ctx.chat.id,
      statusMsg.message_id,
      '⬇️ Загружаю аудио...'
    );
    const audioPath = await downloadAudio(ctx, voice);

    // Определяем целевого пользователя (автора сообщения)
    const msg = ctx.message;
    let targetUserId = ctx.from.id;
    let targetUsername = ctx.from.username || ctx.from.first_name;

    // Проверяем, является ли сообщение пересланным
    if (msg.forward_origin) {
      // Новое API (Grammy 1.20+ / Bot API 7.0)
      if (msg.forward_origin.type === 'user') {
        targetUserId = msg.forward_origin.sender_user.id;
        targetUsername = msg.forward_origin.sender_user.username || msg.forward_origin.sender_user.first_name;
      } else if (msg.forward_origin.type === 'hidden_user') {
        targetUserId = null; // Аватар недоступен
        targetUsername = msg.forward_origin.sender_user_name;
      }
    } else if (msg.forward_from) {
      // Legacy API
      targetUserId = msg.forward_from.id;
      targetUsername = msg.forward_from.username || msg.forward_from.first_name;
    } else if (msg.forward_sender_name) {
      // Скрытый отправитель (Legacy)
      targetUserId = null;
      targetUsername = msg.forward_sender_name;
    }

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
    const avatarPath = await downloadAvatar(ctx, targetUserId);

    // Генерируем видео
    await ctx.api.editMessageText(
      ctx.chat.id,
      statusMsg.message_id,
      '🎬 Создаю видео: 0%'
    );

    let lastUpdatePercent = 0;

    console.log('Target Username for video:', targetUsername);

    const videoPath = await generateVideo(
      audioPath,
      avatarPath,
      transcription,
      targetUsername,
      async (percent) => {
        // Обновляем статус каждые 5%
        if (percent - lastUpdatePercent >= 5 || percent === 100) {
          lastUpdatePercent = percent;
          try {
            await ctx.api.editMessageText(
              ctx.chat.id,
              statusMsg.message_id,
              `🎬 Создаю видео: ${percent}%`
            );
          } catch (e) {
            // Игнорируем ошибки обновления статуса
          }
        }
      }
    );

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
