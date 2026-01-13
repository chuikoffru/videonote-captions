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

  const orderData = pendingInvoices.get(payload);
  if (!orderData) {
    return ctx.reply('❌ Ошибка: файл не найден (возможно, бот был перезагружен). Перешлите сообщение снова.');
  }

  // Удаляем из памяти после успешной оплаты
  pendingInvoices.delete(payload);

  // Создаем объект voice с file_id
  const voice = {
    file_id: orderData.fileId,
    duration: 0
  };

  await ctx.reply('✅ Оплата прошла успешно! Начинаю генерацию видео.');
  await handleVoiceMessage(ctx, voice, true, orderData.author);
});

async function handleVoiceMessage(ctx, voice, isPaid = false, authorOverride = null) {
  // Определяем автора сообщения
  const author = authorOverride || resolveMessageAuthor(ctx);

  // Проверяем длительность и оплату
  if (!isPaid && voice.duration > 60) {
    const duration = voice.duration;
    // Расчет стоимости: 10 звезд за каждые 60 секунд (округление вверх)
    const price = Math.ceil(duration / 60) * 10;

    // Генерируем уникальный ID для payload
    const payloadId = `${ctx.from.id}_${Date.now()}`;

    // Сохраняем file_id и информацию об авторе
    pendingInvoices.set(payloadId, {
      fileId: voice.file_id,
      author: author
    });

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

    // Транскрибируем аудио в текст
    await ctx.api.editMessageText(
      ctx.chat.id,
      statusMsg.message_id,
      '📝 Создаю субтитры...'
    );
    const transcription = await transcribeAudio(audioPath);

    // Скачиваем аватарку (автора или канала)
    await ctx.api.editMessageText(
      ctx.chat.id,
      statusMsg.message_id,
      '🖼️ Загружаю аватар...'
    );
    const avatarPath = await downloadAvatar(ctx, author.id, author.type);

    // Генерируем видео
    await ctx.api.editMessageText(
      ctx.chat.id,
      statusMsg.message_id,
      '🎬 Создаю видео: 0%'
    );

    let lastUpdatePercent = 0;

    console.log('Video Target:', author.username);

    const videoPath = await generateVideo(
      audioPath,
      avatarPath,
      transcription,
      author.username, // Используем имя автора
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

/**
 * Определяет автора сообщения (или исходного автора, если сообщение переслано)
 * Поддерживает User, Hidden User, Channel, Chat и Legacy API
 */
function resolveMessageAuthor(ctx) {
  const msg = ctx.message;

  // 1. Проверяем forward_origin (Bot API 7.0+)
  if (msg.forward_origin) {
    const origin = msg.forward_origin;
    switch (origin.type) {
      case 'user':
        return {
          id: origin.sender_user.id,
          username: origin.sender_user.username || origin.sender_user.first_name,
          type: 'user'
        };
      case 'hidden_user':
        return {
          id: null,
          username: origin.sender_user_name,
          type: 'hidden_user'
        };
      case 'chat':
        return {
          id: origin.sender_chat.id,
          username: origin.sender_chat.title,
          type: 'chat'
        };
      case 'channel':
        return {
          id: origin.chat.id,
          username: origin.chat.title,
          type: 'channel'
        };
    }
  }

  // 2. Legacy API
  if (msg.forward_from) {
    return {
      id: msg.forward_from.id,
      username: msg.forward_from.username || msg.forward_from.first_name,
      type: 'user'
    };
  }
  if (msg.forward_sender_name) {
    return {
      id: null,
      username: msg.forward_sender_name,
      type: 'hidden_user'
    };
  }
  if (msg.forward_from_chat) {
    return {
      id: msg.forward_from_chat.id,
      username: msg.forward_from_chat.title,
      type: msg.forward_from_chat.type === 'channel' ? 'channel' : 'chat'
    };
  }

  // 3. Если не переслано - берем текущего отправителя
  return {
    id: ctx.from.id,
    username: ctx.from.username || ctx.from.first_name,
    type: 'user'
  };
}
