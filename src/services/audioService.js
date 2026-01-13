const fs = require('fs');
const path = require('path');
const axios = require('axios');

/**
 * Скачивает голосовое сообщение из Telegram
 * @param {Object} ctx - Grammy context
 * @param {Object} voice - Voice message object
 * @returns {Promise<string>} Путь к скачанному аудиофайлу
 */
async function downloadAudio(ctx, voice) {
  const file = await ctx.api.getFile(voice.file_id);
  const fileUrl = `https://api.telegram.org/file/bot${process.env.BOT_TOKEN}/${file.file_path}`;
  
  const tempDir = path.join(__dirname, '../../temp');
  if (!fs.existsSync(tempDir)) {
    fs.mkdirSync(tempDir, { recursive: true });
  }
  
  const audioPath = path.join(tempDir, `audio_${Date.now()}.ogg`);
  
  const response = await axios({
    method: 'get',
    url: fileUrl,
    responseType: 'stream'
  });
  
  const writer = fs.createWriteStream(audioPath);
  response.data.pipe(writer);
  
  return new Promise((resolve, reject) => {
    writer.on('finish', () => resolve(audioPath));
    writer.on('error', reject);
  });
}

module.exports = { downloadAudio };
