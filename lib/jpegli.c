#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#include "jpegli/decode.h"
#include "jpegli/encode.h"

#define ALIGN_SIZE 16
#define ALIGNM(x)  ((((x) + ((ALIGN_SIZE) - 1)) / (ALIGN_SIZE)) * (ALIGN_SIZE))

int decode(uint8_t *jpeg_in, int jpeg_in_size, int config_only, uint32_t *width, uint32_t *height, uint32_t *colorspace, uint32_t *chroma, uint8_t *out,
        int fancy_upsampling, int block_smoothing, int arith_code, int dct_method, int tw, int th);

uint8_t* encode(uint8_t *in, int width, int height, int colorspace, int chroma, size_t *size, int quality, int progressive_level, int optimize_coding,
        int adaptive_quantization, int standard_quant_tables, int fancy_downsampling, int dct_method);

typedef enum {
    Y,
    Cb,
    Cr
} planes;

typedef enum {
    YCbCr444,
    YCbCr422,
    YCbCr420,
    YCbCr440,
    YCbCr411,
    YCbCr410
} chroma;

void error_exit(j_common_ptr info) {
  (*info->err->output_message)(info);
}

int decode(uint8_t *jpeg_in, int jpeg_in_size, int config_only, uint32_t *width, uint32_t *height, uint32_t *colorspace, uint32_t *chroma, uint8_t *out,
        int fancy_upsampling, int block_smoothing, int arith_code, int dct_method, int tw, int th) {

    struct jpeg_decompress_struct dinfo;

    struct jpeg_error_mgr jerr;
    dinfo.err = jpegli_std_error(&jerr);
    dinfo.err->error_exit = error_exit;

    jpegli_create_decompress(&dinfo);
    jpegli_mem_src(&dinfo, jpeg_in, jpeg_in_size);

    jpegli_save_markers(&dinfo, JPEG_APP0+13, 0xffff);
    if(jpegli_read_header(&dinfo, 1) != JPEG_HEADER_OK) {
        jpegli_destroy_decompress(&dinfo);
        return 0;
    }

    int scale = 0;
    if(tw > 0 && th > 0) {
        int scale_factor;
        for(scale_factor = 1; scale_factor <= 8; scale_factor++) {
            if(((scale_factor*dinfo.image_width+7)/8) >= tw && ((scale_factor*dinfo.image_height+7)/8) >= th) {
                break;
            }
        }

        if(scale_factor < 8) {
            dinfo.scale_num = scale_factor;
            dinfo.scale_denom = 8;

            scale = 1;
            jpegli_calc_output_dimensions(&dinfo);

            *width = dinfo.output_width;
            *height = dinfo.output_height;
        }
    } else {
        *width = dinfo.image_width;
        *height = dinfo.image_height;
    }

    *colorspace = dinfo.jpeg_color_space;

    int cDiv = 1;
    int subsampleRatio = -1;
    int w, h, cw, ch;

    int forceRGBA = 0;

    switch(dinfo.jpeg_color_space) {
        case JCS_GRAYSCALE:
            if(scale) {
                break;
            }

            dinfo.raw_data_out = 1;
            break;
        case JCS_YCbCr:
            if(scale) {
                break;
            }

            jpeg_saved_marker_ptr marker = dinfo.marker_list;
            while(marker) {
	            if(marker->marker == JPEG_APP0+13) {
                    if(marker->data_length > 0) {
                        forceRGBA = 1;
                        break;
                    }
	            }
	            marker = marker->next;
            }

            dinfo.raw_data_out = 1;

            w = dinfo.comp_info[Y].downsampled_width;
            h = dinfo.comp_info[Y].downsampled_height;
            cw = dinfo.comp_info[Cb].downsampled_width;
            ch = dinfo.comp_info[Cb].downsampled_height;

            if(w == cw && h == ch) {
                subsampleRatio = YCbCr444;

                w = ALIGNM(w);
                h = ALIGNM(h);
                cw = w;
                ch = h;
            } else if(w == cw && (h+1)/2 == ch) {
                subsampleRatio = YCbCr440;
                cDiv = 2;

                w = ALIGNM(w);
                h = ALIGNM(h);
                cw = w;
                ch = (h+1)/2;
            } else if((w+1)/2 == cw && h == ch) {
                subsampleRatio = YCbCr422;

                w = ALIGNM(w);
                h = ALIGNM(h);
                cw = (w+1)/2;
                ch = h;
            } else if((w+1)/2 == cw && (h+1)/2 == ch) {
                if((dinfo.comp_info[Cr].h_samp_factor == 1 && dinfo.comp_info[Cr].v_samp_factor == 2) || \
                        (dinfo.comp_info[Cr].h_samp_factor == 2 && dinfo.comp_info[Cr].v_samp_factor == 2)) {
                    dinfo.raw_data_out = 0;
                    forceRGBA = 1;
                    break;
                }

                subsampleRatio = YCbCr420;
                cDiv = 2;

                w = ALIGNM(w);
                h = ALIGNM(h);
                cw = (w+1)/2;
                ch = (h+1)/2;
            }

            break;
        case JCS_RGB:
            dinfo.out_color_space = JCS_EXT_RGBA;
            break;
        case JCS_CMYK:
        case JCS_YCCK:
            dinfo.out_color_space = JCS_CMYK;
            break;
        default:
            jpegli_destroy_decompress(&dinfo);
            return 0;
    }

    *chroma = subsampleRatio;

    if((scale && (dinfo.jpeg_color_space == JCS_GRAYSCALE || dinfo.jpeg_color_space == JCS_YCbCr)) || forceRGBA) {
        dinfo.out_color_space = JCS_EXT_RGBA;
        *colorspace = JCS_RGB;
    }

    if(config_only) {
        jpegli_destroy_decompress(&dinfo);
        return 1;
    }

    dinfo.dct_method = dct_method;
    dinfo.do_fancy_upsampling = fancy_upsampling;
    dinfo.do_block_smoothing = block_smoothing;
    dinfo.arith_code = arith_code;

    int stride, y_stride, c_stride;
    uint8_t* y_out;
    uint8_t* cb_out;
    uint8_t* cr_out;
    int mcu_rows = 0;

    JSAMPROW row[1];
    JSAMPROW *rows = NULL;
    JSAMPROW *y_rows = NULL;
    JSAMPROW *cb_rows = NULL;
    JSAMPROW *cr_rows = NULL;

    jpegli_set_output_format(&dinfo, JPEGLI_TYPE_UINT8, JPEGLI_NATIVE_ENDIAN);

    if(!jpegli_start_decompress(&dinfo)) {
        jpegli_destroy_decompress(&dinfo);
        return 0;
    }

    mcu_rows = DCTSIZE * dinfo.max_v_samp_factor;
    stride = dinfo.output_width * dinfo.out_color_components;
    
    if(dinfo.jpeg_color_space == JCS_GRAYSCALE && !scale && !forceRGBA) {
        rows = malloc(sizeof(JSAMPROW) * mcu_rows);
    } else if(dinfo.jpeg_color_space == JCS_YCbCr && !scale && !forceRGBA) {
        y_rows = malloc(sizeof(JSAMPROW) * mcu_rows);
        cb_rows = malloc(sizeof(JSAMPROW) * mcu_rows);
        cr_rows = malloc(sizeof(JSAMPROW) * mcu_rows);

        y_stride = w;
        c_stride = cw;

        int i0 = w * h + 0*cw*ch;
        int i1 = w * h + 1*cw*ch;

        y_out = &out[0];
        cb_out = &out[i0];
        cr_out = &out[i1];
    }

    while(dinfo.output_scanline < dinfo.output_height) {
        if(dinfo.jpeg_color_space == JCS_GRAYSCALE && !scale) {
            for(int i = 0; i < mcu_rows; i++) {
                rows[i] = &out[dinfo.output_scanline * stride + (stride * i)];
            }

            jpegli_read_raw_data(&dinfo, &rows, mcu_rows);
        } else if(dinfo.jpeg_color_space == JCS_YCbCr && !scale && !forceRGBA) {
            for(int i = 0; i < mcu_rows; i++) {
                y_rows[i] = &y_out[(dinfo.output_scanline * y_stride) + (y_stride * i)];
                cb_rows[i] = &cb_out[((dinfo.output_scanline * c_stride) / cDiv) + (c_stride * i)];
                cr_rows[i] = &cr_out[((dinfo.output_scanline * c_stride) / cDiv) + (c_stride * i)];
            }
    
            JSAMPARRAY image[] = {y_rows, cb_rows, cr_rows};
            jpegli_read_raw_data(&dinfo, image, mcu_rows);
        } else {
            row[0] = &out[dinfo.output_scanline * stride];

            jpegli_read_scanlines(&dinfo, row, 1);
        }
    }

    if(dinfo.jpeg_color_space == JCS_GRAYSCALE && !scale && !forceRGBA) {
        free(rows);
    } else if(dinfo.jpeg_color_space == JCS_YCbCr && !scale && !forceRGBA) {
        free(y_rows);
        free(cb_rows);
        free(cr_rows);
    }

    if(!jpegli_finish_decompress(&dinfo)) {
        jpegli_destroy_decompress(&dinfo);
        return 0;
    }

    jpegli_destroy_decompress(&dinfo);
    return 1;
}

uint8_t* encode(uint8_t *in, int width, int height, int colorspace, int chroma, size_t *size, int quality, int progressive_level, int optimize_coding,
        int adaptive_quantization, int standard_quant_tables, int fancy_downsampling, int dct_method) {

    struct jpeg_compress_struct cinfo;

    struct jpeg_error_mgr jerr;
    cinfo.err = jpegli_std_error(&jerr);
    cinfo.err->error_exit = error_exit;

    jpegli_create_compress(&cinfo);

    int stride, y_stride, c_stride;
    int w, h, cw, ch;
    int y_h = 0, c_h = 0;
    int cDiv = 1;
    uint8_t* y_in;
    uint8_t* cb_in;
    uint8_t* cr_in;

    JSAMPROW row[1];
    JSAMPROW *rows = NULL;
    JSAMPROW *y_rows = NULL;
    JSAMPROW *cb_rows = NULL;
    JSAMPROW *cr_rows = NULL;

    cinfo.image_width = width;
    cinfo.image_height = height;

    jpegli_set_input_format(&cinfo, JPEGLI_TYPE_UINT8, JPEGLI_NATIVE_ENDIAN);

    if(standard_quant_tables) {
        jpegli_use_standard_quant_tables(&cinfo);
    }

    switch(colorspace) {
        case JCS_GRAYSCALE:
            cinfo.input_components = 1;
            cinfo.in_color_space = JCS_GRAYSCALE;
            jpegli_set_defaults(&cinfo);

            cinfo.raw_data_in = 1;
            cinfo.comp_info[0].h_samp_factor = 1, cinfo.comp_info[0].v_samp_factor = 1;
            break;
        case JCS_YCbCr:
            cinfo.input_components = 3;
            cinfo.in_color_space = JCS_YCbCr;
            jpegli_set_defaults(&cinfo);
        
            cinfo.raw_data_in = 1;
            switch(chroma) {
                case YCbCr444:
                    cinfo.comp_info[Y].h_samp_factor = 1, cinfo.comp_info[Y].v_samp_factor = 1;
                    break;
                case YCbCr440:
                    cinfo.comp_info[Y].h_samp_factor = 1, cinfo.comp_info[Y].v_samp_factor = 2;
                    cDiv = 2;
                    break;
                case YCbCr422:
                    cinfo.comp_info[Y].h_samp_factor = 2, cinfo.comp_info[Y].v_samp_factor = 1;
                    break;
                case YCbCr420:
                    cinfo.comp_info[Y].h_samp_factor = 2, cinfo.comp_info[Y].v_samp_factor = 2;
                    cDiv = 2;
                    break;
            }
            
            for(int i = 1; i < cinfo.num_components; i++) {
                cinfo.comp_info[i].h_samp_factor = 1;
                cinfo.comp_info[i].v_samp_factor = 1;
            }

            break;
        case JCS_RGB:
            cinfo.input_components = 4;
            cinfo.in_color_space = JCS_EXT_RGBA;
            jpegli_set_defaults(&cinfo);

            switch(chroma) {
                case YCbCr444:
                    cinfo.comp_info[Y].h_samp_factor = 1, cinfo.comp_info[Y].v_samp_factor = 1;
                    break;
                case YCbCr440:
                    cinfo.comp_info[Y].h_samp_factor = 1, cinfo.comp_info[Y].v_samp_factor = 2;
                    cDiv = 2;
                    break;
                case YCbCr422:
                    cinfo.comp_info[Y].h_samp_factor = 2, cinfo.comp_info[Y].v_samp_factor = 1;
                    break;
                case YCbCr420:
                    cinfo.comp_info[Y].h_samp_factor = 2, cinfo.comp_info[Y].v_samp_factor = 2;
                    cDiv = 2;
                    break;
            }
            
            for(int i = 1; i < cinfo.num_components; i++) {
                cinfo.comp_info[i].h_samp_factor = 1;
                cinfo.comp_info[i].v_samp_factor = 1;
            }

            break;
        case JCS_CMYK:
            cinfo.input_components = 4;
            cinfo.in_color_space = JCS_CMYK;
            jpegli_set_defaults(&cinfo);
            break;
        default:
            jpegli_destroy_compress(&cinfo);
            return 0;
    }

    float distance = jpegli_quality_to_distance(quality);
    jpegli_set_distance(&cinfo, distance, 1);

    jpegli_set_progressive_level(&cinfo, progressive_level);
    jpegli_enable_adaptive_quantization(&cinfo, adaptive_quantization);

    if(optimize_coding) {
        cinfo.optimize_coding = 1;
    }

    cinfo.dct_method = dct_method;
    cinfo.do_fancy_downsampling = fancy_downsampling;

    uint8_t* out = NULL;
    jpegli_mem_dest(&cinfo, &out, size);

    jpegli_start_compress(&cinfo, 1);

    if(colorspace == JCS_GRAYSCALE) {
        h = DCTSIZE * cinfo.comp_info[0].v_samp_factor;
        rows = malloc(sizeof(JSAMPROW) * h);
    } else if(colorspace == JCS_YCbCr) {
        y_h = DCTSIZE * cinfo.comp_info[Y].v_samp_factor;
        c_h = DCTSIZE * cinfo.comp_info[Cb].v_samp_factor;

        y_rows = malloc(sizeof(JSAMPROW) * y_h);
        cb_rows = malloc(sizeof(JSAMPROW) * c_h);
        cr_rows = malloc(sizeof(JSAMPROW) * c_h);

        switch(chroma) {
            case YCbCr444:
                w = ALIGNM(cinfo.comp_info[Y].downsampled_width);
                h = ALIGNM(cinfo.comp_info[Y].downsampled_height);
                cw = w;
                ch = h;
                break;
            case YCbCr440:
                w = ALIGNM(cinfo.comp_info[Y].downsampled_width);
                h = ALIGNM(cinfo.comp_info[Y].downsampled_height);
                cw = w;
                ch = (h+1)/2;
                break;
            case YCbCr422:
                w = ALIGNM(cinfo.comp_info[Y].downsampled_width);
                h = ALIGNM(cinfo.comp_info[Y].downsampled_height);
                cw = (w+1)/2;
                ch = h;
                break;
            case YCbCr420:
                w = ALIGNM(cinfo.comp_info[Y].downsampled_width);
                h = ALIGNM(cinfo.comp_info[Y].downsampled_height);
                cw = (w+1)/2;
                ch = (h+1)/2;
                break;
        }

        y_stride = w;
        c_stride = cw;

        int i0 = w * h + 0*cw*ch;
        int i1 = w * h + 1*cw*ch;

        y_in = &in[0];
        cb_in = &in[i0];
        cr_in = &in[i1];
    }

    stride = cinfo.image_width * cinfo.input_components;

    while(cinfo.next_scanline < cinfo.image_height) {
        if(colorspace == JCS_GRAYSCALE) {
            for(int i = 0; i < h; i++) {
                rows[i] = &in[cinfo.next_scanline * stride + (stride * i)];
            }

            jpegli_write_raw_data(&cinfo, &rows, h);
        } else if(colorspace == JCS_YCbCr) {
            for(int i = 0; i < y_h; i++) {
                y_rows[i] = &y_in[(cinfo.next_scanline * y_stride) + (y_stride * i)];
            }
            for(int i = 0; i < c_h; i++) {
                cb_rows[i] = &cb_in[((cinfo.next_scanline * c_stride) / cDiv) + (c_stride * i)];
                cr_rows[i] = &cr_in[((cinfo.next_scanline * c_stride) / cDiv) + (c_stride * i)];
            }
    
            JSAMPARRAY image[] = {y_rows, cb_rows, cr_rows};
            jpegli_write_raw_data(&cinfo, image, y_h);
        } else {
            row[0] = &in[cinfo.next_scanline * stride];

            jpegli_write_scanlines(&cinfo, row, 1);
        }
    }

    if(colorspace == JCS_GRAYSCALE) {
        free(rows);
    } else if(colorspace == JCS_YCbCr) {
        free(y_rows);
        free(cb_rows);
        free(cr_rows);
    }

    jpegli_finish_compress(&cinfo);
    jpegli_destroy_compress(&cinfo);

    return out;
}
