const fs = require('fs');
const path = require('path');
const axios = require('axios');

/**
 * Скачивает аватар пользователя из Telegram
 * @param {Object} ctx - Grammy context
 * @returns {Promise<string>} Путь к скачанному аватару
 */
async function downloadAvatar(ctx) {
  const tempDir = path.join(__dirname, '../../temp');
  if (!fs.existsSync(tempDir)) {
    fs.mkdirSync(tempDir, { recursive: true });
  }

  try {
    // Получаем фото профиля пользователя
    const userPhotos = await ctx.api.getUserProfilePhotos(ctx.from.id, { limit: 1 });
    
    if (userPhotos.total_count === 0) {
      // Если у пользователя нет аватарки, используем дефолтную
      return await createDefaultAvatar(tempDir);
    }

    const photo = userPhotos.photos[0][0]; // Берем самое маленькое фото
    const file = await ctx.api.getFile(photo.file_id);
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
  ctx.fillStyle = 'white';
  ctx.font = 'bold 80px Arial';
  ctx.textAlign = 'center';
  ctx.textBaseline = 'middle';
  ctx.fillText('👤', 100, 100);
  
  const avatarPath = path.join(tempDir, `avatar_default_${Date.now()}.png`);
  const buffer = canvas.toBuffer('image/png');
  fs.writeFileSync(avatarPath, buffer);
  
  return avatarPath;
}

module.exports = { downloadAvatar };
