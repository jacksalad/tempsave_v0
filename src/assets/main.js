$(document).ready(function () {
    // 刷新上传的文件列表
    function refreshFileList() {
        $.get('/files', function (data) {
            $('#fileList').children().slice(1).remove();
            data.forEach(function (file) {
                $('#fileList').append(`
                <tr class="table-light">
                    <td class="myalign"><a href="./assets/uploads/${file.name}" target="blank">${file.name}</a></td>
                    <td class="myalign">${file.time}</td>
                    <td class="myalign">${file.size}</td>
                    <td class="myalign"><button fname="${file.name}" type="button" class="btn btn-outline-danger btn-sm delbtn">删除</button></td>
                </tr>
                `)
            });
            $(".delbtn").click(function () {
                $.ajax({
                    url: '/del?name=' + $(this).attr("fname"),
                    type: 'GET',
                    processData: false,
                    contentType: false,
                    success: function (data) {
                        refreshFileList();
                    }
                });
            })
        });
    }

    // 阻止浏览器默认打开文件的行为
    $(document).on('drop dragover', function (e) {
        e.preventDefault();
    });

    // 点击矩形区域触发文件选择
    $('#drop-area').on('click', function () {
        $('#fileInput').click();
    });

    // 当文件拖拽至矩形区域时
    $('#drop-area').on('drop', function (e) {
        $("#progresscol").show();
        e.preventDefault();
        var files = e.originalEvent.dataTransfer.files; // 获取拖拽的文件列表
        var file = files[0]; // 获取文件
        var chunkSize = 1024 * 1024; // 切片大小，这里以1MB为例
        var chunks = Math.ceil(file.size / chunkSize); // 计算需要切成多少片
        var currentChunk = 0;
        uploadChunk(file, chunkSize, chunks, currentChunk); // 开始上传
    });

    // 监听文件选择事件
    $('#fileInput').on('change', function () {
        $('#fileInput').show();

    });

    // 点击上传按钮
    $('#uploadForm').submit(function (e) {
        $("#progresscol").show();
        e.preventDefault();
        var files = $('#fileInput')[0].files;
        var file = files[0]; // 获取文件
        var chunkSize = 1024 * 1024; // 切片大小，这里以1MB为例
        var chunks = Math.ceil(file.size / chunkSize); // 计算需要切成多少片
        var currentChunk = 0;
        uploadChunk(file, chunkSize, chunks, currentChunk);
    });

    // 分块上传
    function uploadChunk(file, chunkSize, chunks, currentChunk) {
        var start = currentChunk * chunkSize;
        var end = Math.min(file.size, start + chunkSize);
        var chunk = file.slice(start, end); // 切片

        var formData = new FormData();
        formData.append('file', chunk);
        formData.append('fileName', file.name);
        formData.append('chunk', currentChunk);
        formData.append('chunks', chunks);

        $.ajax({
            url: '/upload', // 服务器端的上传处理文件地址
            type: 'POST',
            data: formData,
            processData: false,
            contentType: false,
            xhr: function () {
                var xhr = $.ajaxSettings.xhr();
                if (xhr.upload) {
                    xhr.upload.addEventListener('progress', function (e) {
                        if (e.lengthComputable) {
                            var percentComplete = ((e.loaded / e.total) + currentChunk) / chunks;
                            $('#uploadProgress').css('width', parseInt(percentComplete * 100) + '%').text(parseInt(percentComplete * 100) + '%');
                        }
                    }, false);
                }
                return xhr;
            },
            success: function (response) {
                currentChunk++;
                if (currentChunk < chunks) {
                    uploadChunk(file, chunkSize, chunks, currentChunk); // 递归上传下一片
                } else {
                    setTimeout(refreshFileList, 2000);
                    setTimeout(() => {
                        $('#fileInput').hide();
                        $('#progresscol').hide();
                        $('#uploadProgress').css('width', '0%').text('0%');
                    }, 1000);
                    console.log('上传完成');
                }
            },
            error: function () {
                console.error('上传失败');
            }
        });
    }

    refreshFileList();
}) 