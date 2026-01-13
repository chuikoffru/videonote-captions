const fs = require('fs');
const path = require('path');
const axios = require('axios');

/**
 * Скачивает аватар пользователя из Telegram
 * @param {Object} ctx - Grammy context
 * @param {number} userId - ID пользователя (опционально)
 * @returns {Promise<string>} Путь к скачанному аватару
 */
/**
 * Скачивает аватар пользователя или фото чата/канала
 * @param {Object} ctx - Grammy context
 * @param {number|string} targetId - ID пользователя или чата
 * @param {string} type - Тип сущности ('user', 'chat', 'channel')
 * @returns {Promise<string>} Путь к скачанному аватару
 */
async function downloadAvatar(ctx, targetId, type = 'user') {
  const tempDir = path.join(__dirname, '../../temp');
  if (!fs.existsSync(tempDir)) {
    fs.mkdirSync(tempDir, { recursive: true });
  }

  // Если ID не передан, сразу возвращаем дефолтную аватарку
  if (!targetId) {
    return await createDefaultAvatar(tempDir);
  }

  try {
    let fileId;

    if (type === 'chat' || type === 'channel') {
      const chat = await ctx.api.getChat(targetId);
      if (chat.photo) {
        fileId = chat.photo.big_file_id;
      }
    } else {
      // type === 'user'
      const userPhotos = await ctx.api.getUserProfilePhotos(targetId, { limit: 1 });
      if (userPhotos.total_count > 0) {
        fileId = userPhotos.photos[0][0].file_id; // Берем самое маленькое фото для скорости (или [0][1] для качества?)
        // Поправка: [0][0] - это small, [0][1] - big. Для видео лучше big? 
        // В оригинале было [0][0]. Давайте возьмем самое большое доступное.
        const photos = userPhotos.photos[0];
        fileId = photos[photos.length - 1].file_id;
      }
    }

    if (!fileId) {
      return await createDefaultAvatar(tempDir);
    }

    const file = await ctx.api.getFile(fileId);
    const fileUrl = `https://api.telegram.org/file/bot${process.env.BOT_TOKEN}/${file.file_path}`;

    const avatarPath = path.join(tempDir, `avatar_${Date.now()}.jpg`);

    const response = await axios({
      method: 'get',
      url: fileUrl,
      responseType: 'stream'
    });

    const writer = fs.createWriteStream(avatarPath);
    response.data.pipe(writer);

    return new Promise((resolve, reject) => {
      writer.on('finish', () => resolve(avatarPath));
      writer.on('error', reject);
    });
  } catch (error) {
    console.error('Ошибка загрузки аватара:', error);
    return await createDefaultAvatar(tempDir);
  }
}

/**
 * Создает дефолтную аватарку (цветной квадрат)
 * @param {string} tempDir - Директория для временных файлов
 * @returns {Promise<string>} Путь к созданной аватарке
 */
async function createDefaultAvatar(tempDir) {
  const Canvas = require('canvas');
  const canvas = Canvas.createCanvas(200, 200);
  const ctx = canvas.getContext('2d');

  // Создаем градиентный фон
  const gradient = ctx.createLinearGradient(0, 0, 200, 200);
  gradient.addColorStop(0, '#00ffff');
  gradient.addColorStop(1, '#ff00ff');
  ctx.fillStyle = gradient;
  ctx.fillRect(0, 0, 200, 200);

  // Рисуем иконку пользователя
  // Рисуем иконку пользователя (силуэт) вместо эмодзи, чтобы не зависеть от шрифтов
  ctx.fillStyle = 'white';

  // Голова
  ctx.beginPath();
  ctx.arc(100, 85, 40, 0, Math.PI * 2);
  ctx.fill();

  // Тело (плечи)
  ctx.beginPath();
  ctx.arc(100, 200, 80, Math.PI, 0); // Полукруг снизу
  ctx.fill();

  const avatarPath = path.join(tempDir, `avatar_default_${Date.now()}.png`);
  const buffer = canvas.toBuffer('image/png');
  fs.writeFileSync(avatarPath, buffer);

  return avatarPath;
}

module.exports = { downloadAvatar };
